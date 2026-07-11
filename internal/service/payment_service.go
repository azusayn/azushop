package service

import (
	"github.com/azusayn/azushop/internal/common"

	orderpb "github.com/azusayn/azushop/proto/api/order/v1"
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
)

type PaymentService struct {
	pb.UnimplementedPaymentServiceServer
	uc               *biz.PaymentUsecase
	stripeSuccessUrl string
	order            orderpb.OrderServiceClient
}

func NewPaymentService(uc *biz.PaymentUsecase, config *conf.Data) (*PaymentService, error) {
	orderClient, err := NewOrderClient(config)
	if err != nil {
		return nil, err
	}

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
		order:            orderClient,
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
	resp, err := orderService.GetOrder(ctx, &orderpb.GetOrderRequest{OrderId: r.OrderId})
	if err != nil {
		return nil, err
	}

	switch resp.GetOrder().GetOrderStatus() {
	case orderpb.OrderStatus_ORDER_STATUS_PENDING:
		break
	case orderpb.OrderStatus_ORDER_STATUS_CANCELLED:
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("order %d has been cancelled", r.OrderId))
	default:
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("order %d has been paid already", r.OrderId))
	}

	paymentItems, err := convertToPaymentItems(resp.GetOrder().GetOrderItems())
	if err != nil {
		return nil, err
	}
	url, err := h.paymentService.uc.CreatePayment(ctx, r.OrderId, userID, method, paymentItems, h.paymentService.stripeSuccessUrl)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.CreatePaymentResponse{Url: url}), nil
}

func (h *PaymentServiceConnectHandler) Callback(ctx context.Context, req *connect.Request[pb.CallbackRequest]) (*connect.Response[pb.CallbackResponse], error) {
	r := req.Msg
	paymentMethod, err := convertProviderToBizPaymentMethod(r.Provider)
	if err != nil {
		return nil, err
	}
	if err := h.paymentService.uc.Callback(ctx, paymentMethod, r.GetRaw().GetData()); err != nil {
		return nil, status.Error(codes.Internal, codes.Internal.String())
	}
	return connect.NewResponse(&pb.CallbackResponse{}), status.Error(codes.OK, codes.OK.String())
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
