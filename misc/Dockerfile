FROM golang:1.26.6-alpine3.24 AS builder
WORKDIR /build
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/ ./cmd/...

FROM alpine:3.24 AS backend
WORKDIR /app
RUN addgroup --gid 777 --system azushop && adduser --uid 777 --system azushop

COPY --from=builder /build/bin/ /usr/local/bin/

RUN chown -R azushop:azushop /app

CMD ["echo", "please specify the binary to run (e.g. auth, inventory, order, payment, product)"]