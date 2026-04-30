// TODO(0): remember to do idempotency check for all APIs.
package service

import (
	"azushop/internal/conf"
	"azushop/internal/pkg/str"

	authpb "azushop/api/auth/v1"
	inventorypb "azushop/api/inventory/v1"
	orderpb "azushop/api/order/v1"
	paymentpb "azushop/api/payment/v1"
	productpb "azushop/api/product/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ProviderSet is service providers.

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
func newServiceClient(target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
