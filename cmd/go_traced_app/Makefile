.PHONY: build run clean tidy run-multiple

# Build the application
build:
	@echo "Building go-traced-app..."
	go build -o go-traced-app

# Run the application with default settings
run: build
	@echo "Starting go-traced-app..."
	./go-traced-app

# Run multiple instances as different services (in background)
run-multiple: build
	@echo "Starting multiple service instances..."
	@./go-traced-app -service frontend-service &
	@./go-traced-app -service backend-service &
	@./go-traced-app -service api-gateway &
	@echo "Started 3 service instances. Use 'pkill go-traced-app' to stop all."

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f go-traced-app

# Download dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy
	go mod download

# Default target
all: tidy build
