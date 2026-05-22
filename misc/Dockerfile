FROM golang:1.26.3-alpine3.23 AS builder
WORKDIR /build
COPY . .

RUN go mod download
RUN go build -ldflags="-w -s" -o bin/ ./cmd/...

FROM alpine:3.23 AS backend
WORKDIR /app
RUN addgroup --gid 777 --system azushop && adduser --uid 777 --system azushop

COPY --from=builder /build/bin/ /usr/local/bin/
COPY --from=builder /build/configs/ /app/configs/

# TODO: HEALTH CHECK.

CMD ["echo", "please specify the binary to run (e.g. auth, inventory, order, payment, product)"]
