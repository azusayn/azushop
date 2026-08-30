package service

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/azusayn/azushop/internal/common"

	pb "github.com/azusayn/azushop/proto/api/payment/v1"
	"github.com/azusayn/azushop/proto/conf"

	"os"

	"context"
	"errors"
	"fmt"

	"github.com/azusayn/azushop/internal/biz"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"connectrpc.com/connect"

	orderpb "github.com/azusayn/azushop/proto/api/order/v1"
	orderv1connect "github.com/azusayn/azushop/proto/api/order/v1/v1connect"
)

const (
	PaymentProviderPathValue = "provider"
)

type PaymentService struct {
	uc               *biz.PaymentUsecase
	stripeSuccessUrl string
	order            orderv1connect.OrderServiceClient
}

func NewPaymentService(
	uc *biz.PaymentUsecase,
	order orderv1connect.OrderServiceClient,
	config *conf.Data,
) (*PaymentService, error) {
	if secret := os.Getenv("STRIPE_SECRET_KEY"); secret != "" {
		stripe.Key = secret
	} else if config.GetPayment().GetStripeSecretKey(); secret != "" {
		stripe.Key = secret
	} else {
		return nil, errors.New("stripe secret key not configured")
	}

	successURL := config.GetPayment().GetStripeSuccessUrl()
	if successURL == "" {
		return nil, errors.New("stripe success URL not configured")
	}

	return &PaymentService{
		uc:               uc,
		stripeSuccessUrl: successURL,
		order:            order,
	}, nil
}

// PaymentServiceConnectHandler implements the ConnectRPC handler for PaymentService.
type PaymentServiceConnectHandler struct {
	paymentService *PaymentService
}

func NewPaymentServiceConnectHandler(paymentService *PaymentService) *PaymentServiceConnectHandler {
	return &PaymentServiceConnectHandler{paymentService: paymentService}
}

func (h *PaymentServiceConnectHandler) CreatePayment(ctx context.Context, req *connect.Request[pb.CreatePaymentRequest]) (*connect.Response[pb.CreatePaymentResponse], error) {
	idempotencyKey, err := common.ExtractIdempotencyKey(&ctx)
	if err != nil {
		return nil, err
	}

	r := req.Msg
	userID, _, err := common.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	method, err := convertToBizPaymentMethod(r.PaymentMethod)
	if err != nil {
		return nil, err
	}
	orderService := h.paymentService.order
	getReq := connect.NewRequest(&orderpb.GetOrderRequest{OrderId: r.OrderId})
	resp, err := orderService.GetOrder(ctx, getReq)
	if err != nil {
		return nil, err
	}

	switch resp.Msg.GetOrder().GetOrderStatus() {
	case orderpb.OrderStatus_ORDER_STATUS_PENDING:
		break
	case orderpb.OrderStatus_ORDER_STATUS_CANCELLED:
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("order %d has been cancelled", r.OrderId))
	default:
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("order %d has been paid already", r.OrderId))
	}

	paymentItems, err := convertToPaymentItems(resp.Msg.GetOrder().GetOrderItems())
	if err != nil {
		return nil, err
	}
	url, err := h.paymentService.uc.CreatePayment(ctx, idempotencyKey, r.OrderId, userID, method, paymentItems, h.paymentService.stripeSuccessUrl)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.CreatePaymentResponse{Url: url}), nil
}

func NewPaymentCallbackHandler(uc *biz.PaymentUsecase) *PaymentCallbackHandler {
	return &PaymentCallbackHandler{uc: uc}
}

type PaymentCallbackHandler struct {
	uc *biz.PaymentUsecase
}

func (h *PaymentCallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f := func() error {
		if r == nil {
			return errors.New("empty request")
		}

		// TODO(3): payment success page.
		if r.Method != http.MethodPost {
			return nil
		}

		provider := r.PathValue(PaymentProviderPathValue)
		if provider == "" {
			return errors.New("empty provider")
		}
		paymentMethod, err := convertProviderToBizPaymentMethod(provider)
		if err != nil {
			return err
		}
		bytes, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		if err := h.uc.Callback(r.Context(), paymentMethod, bytes); err != nil {
			return err
		}
		return nil
	}

	if err := f(); err != nil {
		slog.ErrorContext(r.Context(), "failed to process provider callback", slog.Any("err", err))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func convertToPaymentItems(orderItems []*orderpb.OrderItem) ([]*biz.PaymentItem, error) {
	var paymentItems []*biz.PaymentItem
	for _, item := range orderItems {
		if item.GetProductName() == "" {
			return nil, errors.New("nil product name")
		}
		if item.UnitPrice == nil {
			return nil, errors.New("nil unit price")
		}
		unitPrice, err := decimal.NewFromString(*item.UnitPrice)
		if err != nil {
			return nil, err
		}
		bytes, err := protojson.Marshal(item.GetAttrs())
		if err != nil {
			return nil, err
		}
		paymentItems = append(paymentItems, &biz.PaymentItem{
			Name:      item.GetProductName(),
			Quantity:  item.GetQuantity(),
			UnitPrice: unitPrice,
			Attr:      bytes,
		})
	}
	return paymentItems, nil
}

func convertToBizPaymentMethod(method pb.PaymentMethod) (biz.PaymentMethod, error) {
	switch method {
	case pb.PaymentMethod_PAYMENT_METHOD_STRIPE:
		return biz.PaymentMethodStripe, nil
	case pb.PaymentMethod_PAYMENT_METHOD_ALIPAY:
		return biz.PaymentMethodAlipay, nil
	case pb.PaymentMethod_PAYMENT_METHOD_WECHAT:
		return biz.PaymentMethodWechat, nil
	default:
	}
	return "", fmt.Errorf("unsupported payment method %q", method)
}

func convertProviderToBizPaymentMethod(provider string) (biz.PaymentMethod, error) {
	switch provider {
	case "stripe":
		return biz.PaymentMethodStripe, nil
	default:
	}
	return "", fmt.Errorf("unsupported payment provider %q", provider)
}
