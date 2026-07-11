# azushop

Go microservices e-commerce platform with product, inventory, order, payment and auth services, built on Kratos with Kafka, Redis and PostgreSQL.

```bash
  # brew install buf
  # go install github.com/google/wire/cmd/wire@latest

  # proto
  buf generate

  # DI 
  wire ./cmd/...
  # or use go 'generate ./...' as a replacement
  
  # run a single service
  go run ./cmd/auth
```
