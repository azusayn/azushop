# azushop

Go microservices e-commerce platform with product, inventory, order, payment and auth services, built on Kratos with Kafka, Redis and PostgreSQL.

```bash
  # go install github.com/google/wire/cmd/wire@latest
  wire ./cmd/auth 
  # or use go 'generate ./cmd/auth' as a replacement
  
  go run ./cmd/auth
```
