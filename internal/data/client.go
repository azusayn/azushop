package data

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/proto/conf"

	authv1connect "github.com/azusayn/azushop/proto/api/auth/v1/v1connect"
	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	orderv1connect "github.com/azusayn/azushop/proto/api/order/v1/v1connect"
	paymentv1connect "github.com/azusayn/azushop/proto/api/payment/v1/v1connect"
	productv1connect "github.com/azusayn/azushop/proto/api/product/v1/v1connect"
)

func newHTTPClient() *http.Client {
	return &http.Client{}
}

func newAuthenticatedClientOptions() ([]connect.ClientOption, error) {
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, err
	}
	return []connect.ClientOption{
		connect.WithInterceptors(
			otelInterceptor,
			middleware.AuthClientInterceptor(),
		),
		connect.WithGRPC(),
	}, nil
}

func newServiceClient[T any](
	newFn func(connect.HTTPClient, string, ...connect.ClientOption) T,
	addr string,
	opts []connect.ClientOption,
) T {
	return newFn(newHTTPClient(), "http://"+addr, opts...)
}

func NewInventoryClient(config *conf.Data) (inventoryv1connect.InventoryServiceClient, error) {
	opts, err := newAuthenticatedClientOptions()
	if err != nil {
		return nil, err
	}
	return newServiceClient(
		inventoryv1connect.NewInventoryServiceClient,
		config.GetServiceAddr().GetInventory(),
		opts,
	), nil
}

func NewProductClient(config *conf.Data) (productv1connect.ProductServiceClient, error) {
	opts, err := newAuthenticatedClientOptions()
	if err != nil {
		return nil, err
	}
	return newServiceClient(
		productv1connect.NewProductServiceClient,
		config.GetServiceAddr().GetProduct(),
		opts,
	), nil
}

func NewOrderClient(config *conf.Data) (orderv1connect.OrderServiceClient, error) {
	opts, err := newAuthenticatedClientOptions()
	if err != nil {
		return nil, err
	}
	return newServiceClient(
		orderv1connect.NewOrderServiceClient,
		config.GetServiceAddr().GetOrder(),
		opts,
	), nil
}

func NewAuthClient(config *conf.Data) (authv1connect.AuthServiceClient, error) {
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, err
	}
	addr := config.GetServiceAddr().GetAuth()
	return authv1connect.NewAuthServiceClient(
		newHTTPClient(),
		"http://"+addr,
		connect.WithInterceptors(otelInterceptor),
		connect.WithGRPC(),
	), nil
}

func NewPaymentClient(config *conf.Data) (paymentv1connect.PaymentServiceClient, error) {
	opts, err := newAuthenticatedClientOptions()
	if err != nil {
		return nil, err
	}
	return newServiceClient(
		paymentv1connect.NewPaymentServiceClient,
		config.GetServiceAddr().GetPayment(),
		opts,
	), nil
}
