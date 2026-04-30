# azushop

Go microservices e-commerce platform with product, inventory, order, payment and auth services, built on Kratos with Kafka, Redis and PostgreSQL.

```bash
  
  # sudo apt install -y protobuf-compiler
  # go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
  # go install github.com/google/wire/cmd/wire@latest

  # proto
  kratos proto client internal/conf/
  kratos proto client api
  kratos proto server api 
  
  wire ./cmd/...
  # or use go 'generate ./...' as a replacement
  
  # run a single service
  go run ./cmd/auth
```
