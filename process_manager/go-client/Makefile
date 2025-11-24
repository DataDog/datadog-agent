.PHONY: proto build test clean example

# Generate Go code from proto files
proto:
	@echo "Generating Go code from proto files..."
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       -I ../proto \
	       ../proto/process_manager.proto
	@echo "Proto files generated successfully"

# Build the client library
build:
	@echo "Building Go client..."
	go build ./...
	@echo "Build successful"

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Build example
example: build
	@echo "Building example..."
	go build -o pm-example ./example/main.go

# Clean generated files
clean:
	@echo "Cleaning..."
	rm -f *.pb.go
	rm -f pm-example
	@echo "Clean complete"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	@echo "Dependencies installed"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Full setup (proto + deps + build)
setup: proto deps build
	@echo "Setup complete"

