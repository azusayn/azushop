go run ./cmd/auth &
# go run ./cmd/order &
go run ./cmd/product &
go run ./cmd/inventory &

wait
echo "all processes exit..."