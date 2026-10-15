package biz

import (
	"context"
	"encoding/json"
	"fmt"

	"uuid"

	"github.com/azusayn/azushop/internal/pkg/db"
	"github.com/azusayn/azushop/internal/pkg/llm"
	"github.com/azusayn/azushop/internal/pkg/outbox"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/azusayn/azutils/validate"
	"github.com/pgvector/pgvector-go"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type ProductStatus string

const (
	ProductStatusUnspecified ProductStatus = "unspecified"
	ProductStatusDraft       ProductStatus = "draft"
	ProductStatusPending     ProductStatus = "pending"
	ProductStatusActive      ProductStatus = "active"
	ProductStatusOffline     ProductStatus = "offline"
)

const productEmbeddingFmt = "Title: %s\nDescription: %s"

type Numeric string

type Sku struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	// map[string]string
	Attrs     json.RawMessage
	UnitPrice Numeric
}

type SkuDetail struct {
	Sku         Sku
	ProductName string
}

type Product struct {
	ID          uuid.UUID
	ProductName string
	SellerID    int32
	Status      ProductStatus
	Skus        []*Sku
	Description string
	Embedding   pgvector.Vector
}

type ProductRepo interface {
	SearchProductsByKeyword(ctx context.Context, embedding []float32, maxDistance float32) ([]*Product, error)
	ListProductsBySellerID(ctx context.Context, sellerID int32, pageToken uuid.UUID, pageSize int32, productStatus ProductStatus) ([]*Product, error)
	BatchCreateProducts(ctx context.Context, product []*Product) ([]*Product, error)
	BatchUpdateProducts(ctx context.Context, product []*Product, paths []string) error
	BatchGetSkuDetails(ctx context.Context, skuIDs []uuid.UUID, pageToken uuid.UUID, pageSize int32) ([]*SkuDetail, error)
	ClearSellerProductCaches(ctx context.Context, sellerIDs []int32)
}

type ProductPublisher interface {
	PublishProductCreated(ctx context.Context, skuIDs []uuid.UUID) error
}

type ProductUsecase struct {
	repo            ProductRepo
	publisher       ProductPublisher
	embeddingClient *llm.OpenAIClient
	tx              *db.Transaction
	outbox          *outbox.Outbox
}

func NewProductUsecase(
	repo ProductRepo,
	publisher ProductPublisher,
	openaiClient *llm.OpenAIClient,
	tx *db.Transaction,
	config *conf.Data,
) *ProductUsecase {
	uc := &ProductUsecase{
		repo:            repo,
		publisher:       publisher,
		embeddingClient: openaiClient,
		tx:              tx,
	}
	uc.outbox = outbox.NewOutbox(config.GetServiceName(), uc.buildDispatchFunc(), tx)
	return uc
}

// used by listSellerProducts()
func productStatusFilter(productStatus ProductStatus, sellerID, userID int32, role UserRole) ProductStatus {
	switch role {
	case UserRoleAdministrator:
		return productStatus

	case UserRoleMerchant:
		if sellerID == userID {
			return productStatus
		}
		return ProductStatusActive

	case UserRoleCustomer:
		return ProductStatusActive

	default:
	}
	return ProductStatusUnspecified
}

// TODO(3): product quantity limits.
func (uc *ProductUsecase) BatchCheckProducts(products []*Product) error {
	nc := validate.NewNameChecker(
		validate.WithLengthLimit(1, 50),
		validate.WithAllowSpace(),
		validate.WithAllowPunct(),
	)
	if len(products) == 0 {
		return errors.New("empty products")
	}
	for _, p := range products {
		if err := nc.BasicCheck(p.ProductName); err != nil {
			return err
		}
		if len(p.Skus) == 0 {
			return fmt.Errorf("empty skus for product %q", p.ProductName)
		}
	}
	return nil
}

// userID is the user calling the API.
func (uc *ProductUsecase) ListSellerProducts(
	ctx context.Context,
	sellerID int32,
	pageToken uuid.UUID,
	// TODO(0): implement a paginator. refer to openai-go SDK.
	pageSize int32,
	productStatus ProductStatus,
	userID int32,
	userRole UserRole,
) ([]*Product, error) {
	productStatus = productStatusFilter(productStatus, sellerID, userID, userRole)
	products, err := uc.repo.ListProductsBySellerID(ctx, sellerID, pageToken, pageSize, productStatus)
	if err != nil {
		return nil, err
	}
	return products, nil
}

func productsFilter(products []*Product, userID int32, role UserRole) ([]*Product, error) {
	switch role {
	case UserRoleAdministrator:
		// TODO(3): pass seller ID in...
		return products, nil
	case UserRoleMerchant:
		for _, p := range products {
			p.SellerID = userID
			p.Status = ProductStatusOffline
		}
		return products, nil
	default:
	}
	return nil, fmt.Errorf("role %q doesn't have the permissions to operate products", role)
}

func (uc *ProductUsecase) SearchProducts(ctx context.Context, keyword string, maxDistance float32, pageToken uuid.UUID, pageSize int32) ([]*Product, error) {
	embeddings, err := uc.embeddingClient.CreateEmbeddings(ctx, []string{keyword})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create embeddings")
	}
	if len(embeddings) != 1 {
		return nil, errors.New("invalid embeddings dimension")
	}
	return uc.repo.SearchProductsByKeyword(ctx, embeddings[0].Slice(), maxDistance)
}

func (uc *ProductUsecase) BatchCreateProducts(
	ctx context.Context,
	products []*Product,
	userID int32,
	userRole UserRole,
) ([]*Product, error) {
	products, err := productsFilter(products, userID, userRole)
	if err != nil {
		return nil, err
	}

	embeddingInputs := lo.Map(products, func(p *Product, _ int) string {
		return fmt.Sprintf(productEmbeddingFmt, p.ProductName, p.Description)
	})

	// generate embedding by product names.
	embeddings, err := uc.embeddingClient.CreateEmbeddings(ctx, embeddingInputs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create embeddings")
	}
	for i, p := range products {
		p.Embedding = embeddings[i]
	}

	var createdProds []*Product
	err = uc.tx.Transaction(ctx, func(ctx context.Context) error {
		var err error
		createdProds, err = uc.repo.BatchCreateProducts(ctx, products)
		if err != nil {
			return err
		}
		var skuIDs []uuid.UUID
		for _, p := range createdProds {
			for _, s := range p.Skus {
				skuIDs = append(skuIDs, s.ID)
			}
		}
		payload, err := json.Marshal(ProductCreatedMessage{SkuIDs: skuIDs})
		if err != nil {
			return err
		}
		return uc.outbox.Save(ctx, string(outboxEventProductCreated), payload)
	})
	if err != nil {
		return nil, err
	}

	seen := make(map[int32]struct{}, len(createdProds))
	sellerIDs := make([]int32, 0, len(createdProds))
	for _, p := range createdProds {
		if _, ok := seen[p.SellerID]; ok {
			continue
		}
		seen[p.SellerID] = struct{}{}
		sellerIDs = append(sellerIDs, p.SellerID)
	}
	uc.repo.ClearSellerProductCaches(ctx, sellerIDs)
	return createdProds, nil
}

func (uc *ProductUsecase) HandleOutboxMessages(ctx context.Context) error {
	return uc.outbox.Run(ctx)
}

func (uc *ProductUsecase) dispatchOutboxMessage(ctx context.Context, message *outbox.OutboxMessage) error {
	switch OutboxEventType(message.EventType) {
	case outboxEventProductCreated:
		var msg ProductCreatedMessage
		if err := json.Unmarshal(message.Payload, &msg); err != nil {
			return err
		}
		return uc.publisher.PublishProductCreated(ctx, msg.SkuIDs)
	default:
	}
	return fmt.Errorf("unknown event type %q", message.EventType)
}

func (uc *ProductUsecase) buildDispatchFunc() outbox.DispatchFunc {
	return func(ctx context.Context, message *outbox.OutboxMessage) error {
		return uc.dispatchOutboxMessage(ctx, message)
	}
}

func (uc *ProductUsecase) BatchUpdateProducts(
	ctx context.Context,
	products []*Product,
	paths []string,
	userID int32,
	userRole UserRole,
) error {
	products, err := productsFilter(products, userID, userRole)
	if err != nil {
		return err
	}
	return uc.repo.BatchUpdateProducts(ctx, products, paths)
}

func (uc *ProductUsecase) BatchGetSkuDetails(ctx context.Context, skuIDs []uuid.UUID, pageToken uuid.UUID, pageSize int32) ([]*SkuDetail, error) {
	return uc.repo.BatchGetSkuDetails(ctx, skuIDs, pageToken, pageSize)
}
