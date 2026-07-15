.PHONY: build test lint run clean proto

BINARY := network-manager
CMD_DIR := ./cmd/network-manager

build:
	go build -o bin/$(BINARY) $(CMD_DIR)

test:
	go test ./... -race -count=1

test-coverage:
	go test ./... -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out

lint:
	golangci-lint run ./...

run:
	go run $(CMD_DIR)

clean:
	rm -rf bin/ coverage.out

docker-build:
	docker build -t $(BINARY):latest .

# Generate proto (requires buf CLI and plugins)
proto:
	buf generate proto
