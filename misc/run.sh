#!/bin/bash
go run ./cmd/auth &
go run ./cmd/order &
go run ./cmd/product &
go run ./cmd/inventory &
go run ./cmd/payment &

wait
echo "All processes have exited."