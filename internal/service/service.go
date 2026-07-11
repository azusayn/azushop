// TODO(0): remember to do idempotency check for all APIs.
package service

import (
	authpb "github.com/azusayn/azushop/proto/api/auth/v1"
	inventorypb "github.com/azusayn/azushop/proto/api/inventory/v1"
	orderpb "github.com/azusayn/azushop/proto/api/order/v1"
	paymentpb "github.com/azusayn/azushop/proto/api/payment/v1"
	productpb "github.com/azusayn/azushop/proto/api/product/v1"
	"github.com/azusayn/azushop/proto/conf"

	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/internal/pkg/str"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func convertToUniquePaths(updateMask *fieldmaskpb.FieldMask) []string {
	ss := str.NewStringSet(str.WithValues(updateMask.GetPaths()))
	return ss.ToSlice()
}

const (
	ServiceNameAuth      = "service.auth"
	ServiceNameOrder     = "service.order"
	ServiceNameInventory = "service.inventory"
	ServiceNameProduct   = "service.product"
	ServiceNamePayment   = "service.payment"
)

// TODO(1): mtls.
func newServiceClient(address string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(
			middleware.AuthClientInterceptor(),
		),
	)
}

func NewInventoryClient(config *conf.Data) (inventorypb.InventoryServiceClient, error) {
	conn, err := newServiceClient(config.GetServiceAddr().GetInventory())
	if err != nil {
		return nil, err
	}
	return inventorypb.NewInventoryServiceClient(conn), err
}

func NewProductClient(config *conf.Data) (productpb.ProductServiceClient, error) {
	conn, err := newServiceClient(config.GetServiceAddr().GetProduct())
	if err != nil {
		return nil, err
	}
	return productpb.NewProductServiceClient(conn), err
}

func NewOrderClient(config *conf.Data) (orderpb.OrderServiceClient, error) {
	conn, err := newServiceClient(config.GetServiceAddr().GetOrder())
	if err != nil {
		return nil, err
	}
	return orderpb.NewOrderServiceClient(conn), nil
}

func NewAuthClient(config *conf.Data) (authpb.AuthServiceClient, error) {
	conn, err := newServiceClient(config.GetServiceAddr().GetAuth())
	if err != nil {
		return nil, err
	}
	return authpb.NewAuthServiceClient(conn), nil
}

func NewPaymentClient(config *conf.Data) (paymentpb.PaymentServiceClient, error) {
	conn, err := newServiceClient(config.GetServiceAddr().GetPayment())
	if err != nil {
		return nil, err
	}
	return paymentpb.NewPaymentServiceClient(conn), nil
}
