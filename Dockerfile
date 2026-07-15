FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /network-manager ./cmd/network-manager

FROM alpine:3.21

RUN apk --no-cache add ca-certificates

COPY --from=builder /network-manager /usr/local/bin/network-manager

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/network-manager"]
