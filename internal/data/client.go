package data

import (
	"net/http"

	"github.com/azusayn/azushop/internal/pkg/middleware"
	"github.com/azusayn/azushop/proto/conf"

	authv1connect "github.com/azusayn/azushop/proto/api/auth/v1/v1connect"
	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	orderv1connect "github.com/azusayn/azushop/proto/api/order/v1/v1connect"
	paymentv1connect "github.com/azusayn/azushop/proto/api/payment/v1/v1connect"
	productv1connect "github.com/azusayn/azushop/proto/api/product/v1/v1connect"

	"connectrpc.com/connect"
)

func newHTTPClient() *http.Client {
	return &http.Client{}
}

func NewInventoryClient(config *conf.Data) inventoryv1connect.InventoryServiceClient {
	addr := config.GetServiceAddr().GetInventory()
	return inventoryv1connect.NewInventoryServiceClient(
		newHTTPClient(),
		"http://"+addr,
		connect.WithInterceptors(middleware.AuthClientInterceptor()),
		connect.WithGRPC(),
	)
}

func NewProductClient(config *conf.Data) productv1connect.ProductServiceClient {
	addr := config.GetServiceAddr().GetProduct()
	return productv1connect.NewProductServiceClient(
		newHTTPClient(),
		"http://"+addr,
		connect.WithInterceptors(middleware.AuthClientInterceptor()),
		connect.WithGRPC(),
	)
}

func NewOrderClient(config *conf.Data) orderv1connect.OrderServiceClient {
	addr := config.GetServiceAddr().GetOrder()
	return orderv1connect.NewOrderServiceClient(
		newHTTPClient(),
		"http://"+addr,
		connect.WithInterceptors(middleware.AuthClientInterceptor()),
		connect.WithGRPC(),
	)
}

func NewAuthClient(config *conf.Data) authv1connect.AuthServiceClient {
	addr := config.GetServiceAddr().GetAuth()
	return authv1connect.NewAuthServiceClient(
		newHTTPClient(),
		"http://"+addr,
		connect.WithGRPC(),
	)
}

func NewPaymentClient(config *conf.Data) paymentv1connect.PaymentServiceClient {
	addr := config.GetServiceAddr().GetPayment()
	return paymentv1connect.NewPaymentServiceClient(
		newHTTPClient(),
		"http://"+addr,
		connect.WithInterceptors(middleware.AuthClientInterceptor()),
		connect.WithGRPC(),
	)
}
