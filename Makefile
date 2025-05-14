.PHONY: build run clean test docker-build docker-run docker-compose-up docker-compose-down

# Build binary
build:
	go build -o bin/rayai-node ./cmd/rayai-node

# Run application
run:
	go run ./cmd/rayai-node

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test -v ./...

# Docker commands
docker-build:
	docker build -t rayai-node .

docker-run:
	docker run -d --name rayai-node -p 8080:8080 -p 6379:6379 -p 8265:8265 rayai-node

docker-compose-up:
	docker-compose up -d

docker-compose-down:
	docker-compose down
