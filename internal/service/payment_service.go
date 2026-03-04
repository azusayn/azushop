package service

import (
	orderpb "azushop/api/order/v1"
	pb "azushop/api/payment/v1"
	"azushop/internal/biz"
	"azushop/internal/data"
	"azushop/internal/pkg/middleware"
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type PaymentService struct {
	pb.UnimplementedPaymentServiceServer
	uc   *biz.PaymentUsecase
	data *data.Data
}

func NewPaymentService(uc *biz.PaymentUsecase, data *data.Data) *PaymentService {
	return &PaymentService{
		uc:   uc,
		data: data,
	}
}

func (s *PaymentService) CreatePayment(ctx context.Context, req *pb.CreatePaymentRequest) (*pb.CreatePaymentResponse, error) {
	userID, _, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	method, err := convertToBizPaymentMethod(req.PaymentMethod)
	if err != nil {
		return nil, err
	}
	orderService := s.data.GetOrderService()
	resp, err := orderService.GetOrder(ctx, &orderpb.GetOrderRequest{OrderId: req.OrderId})
	if err != nil {
		return nil, err
	}
	if resp.GetOrder().GetOrderStatus() != orderpb.OrderStatus_ORDER_STATUS_PENDING {
		return nil, status.Error(codes.OK, "duplicate calls")
	}
	paymentItems, err := convertToPaymentItems(resp.GetOrder().GetOrderItems())
	if err != nil {
		return nil, err
	}
	url, err := s.uc.CreatePayment(ctx, req.OrderId, userID, method, paymentItems, s.data.GetStripeSuccessUrl())
	if err != nil {
		return nil, err
	}
	return &pb.CreatePaymentResponse{Url: url}, nil
}

func (s *PaymentService) Callback(ctx context.Context, req *pb.CallbackRequest) (*pb.CallbackResponse, error) {
	paymentMethod, err := convertProviderToBizPaymentMethod(req.Provider)
	if err != nil {
		return nil, err
	}
	// return an error to trigger a retry from the payment provider.
	if err := s.uc.Callback(ctx, paymentMethod, req.GetRaw().GetData()); err != nil {
		return nil, status.Error(codes.Internal, codes.Internal.String())
	}
	return &pb.CallbackResponse{}, status.Error(codes.OK, codes.OK.String())
}

func convertToPaymentItems(orderItems []*orderpb.OrderItem) ([]*biz.PaymentItem, error) {
	var paymentItems []*biz.PaymentItem
	for _, item := range orderItems {
		if item.UnitPrice == nil {
			return nil, errors.New("failed to fetch unit price from product service")
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
			// TODO(0): product name.
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
