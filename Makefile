.PHONY: run build wire dev clean

# Run the application
run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

# Build the application
build:
	go build -o bin/api cmd/api/*.go

# Generate wire dependencies
wire:
	cd cmd/api && wire
	cd cmd/worker && wire

# Run in development mode
dev: wire run

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Start dependencies (postgres)
docker-up:
	docker start postgres-go-auth || docker run --name postgres-go-auth \
		-e POSTGRES_USER=postgres \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_DB=go-auth \
		-p 5432:5432 \
		-d postgres:latest

# Stop dependencies
docker-down:
	docker stop postgres-go-auth