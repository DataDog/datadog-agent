.PHONY: help build test test-engine test-e2e test-all clean run-daemon stop-daemon
.PHONY: docker-build docker-shell docker-test docker-test-e2e docker-mock docker-down docker-clean
.PHONY: dev-start dev-stop dev-daemon dev-cli

help:
	@echo "Process Manager - Test Suite"
	@echo ""
	@echo "Native targets:"
	@echo "  make build          - Build all binaries (release mode)"
	@echo "  make build-debug    - Build all binaries (debug mode)"
	@echo "  make test           - Run all tests (engine + e2e)"
	@echo "  make test-engine    - Run engine integration tests only"
	@echo "  make test-e2e       - Run e2e tests (daemon + CLI)"
	@echo "  make test-quick     - Run quick smoke tests"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make run-daemon     - Start daemon in background"
	@echo "  make stop-daemon    - Stop running daemon"
	@echo "  make fmt            - Format code"
	@echo "  make clippy         - Run clippy linter"
	@echo "  make ci             - Run full CI pipeline (fmt, clippy, test)"
	@echo ""
	@echo "Docker targets (for macOS testing):"
	@echo "  make docker-build   - Build Docker test image"
	@echo "  make docker-shell   - Get interactive shell in container"
	@echo "  make docker-test    - Run all tests in Docker"
	@echo "  make docker-test-e2e - Run e2e tests in Docker"
	@echo "  make docker-mock    - Test with mock Datadog services"
	@echo "  make docker-down    - Stop all containers"
	@echo "  make docker-clean   - Remove containers and images"
	@echo ""
	@echo "Manual testing (multi-container):"
	@echo "  make dev-start      - Start dev environment (daemon + cli containers)"
	@echo "  make dev-daemon     - Get shell in daemon container"
	@echo "  make dev-cli        - Get shell in CLI container"
	@echo "  make dev-stop       - Stop dev environment"

# Build targets
build:
	@echo "🔨 Building release binaries..."
	cargo build --release
	@echo "✅ Build complete: target/release/{daemon,cli}"

build-debug:
	@echo "🔨 Building debug binaries..."
	cargo build
	@echo "✅ Build complete: target/debug/{daemon,cli}"

# Test targets
test-engine:
	@echo "🧪 Running engine integration tests..."
	cd engine && cargo test --lib --tests
	@echo "✅ Engine tests passed"

test-e2e: build
	@echo "🧪 Running e2e tests in parallel..."
	@echo "⚠️  Note: E2E tests will start/stop daemon automatically"
	@echo "💡 If tests fail with 'Address already in use', run: pkill -9 -f 'target/release/daemon'"
	cd tests && cargo test
	@echo "✅ E2E tests passed"

test-all: test-engine test-e2e
	@echo "🎉 All tests passed!"

test: test-all

test-quick:
	@echo "⚡ Running quick smoke tests..."
	cd engine && cargo test --test engine_lifecycle -- --test-threads=1
	@echo "✅ Quick tests passed"

# Code quality
fmt:
	@echo "🎨 Formatting code..."
	cargo fmt --all
	@echo "✅ Code formatted"

fmt-check:
	@echo "🔍 Checking code format..."
	cargo fmt --all -- --check

clippy:
	@echo "📎 Running clippy..."
	cargo clippy --all-targets --all-features -- -D warnings
	@echo "✅ Clippy checks passed"

# Daemon management
run-daemon:
	@echo "🚀 Starting daemon..."
	@pkill -9 -f "target/debug/daemon" || true
	@pkill -9 -f "target/release/daemon" || true
	@sleep 1
	./target/debug/daemon &
	@sleep 2
	@echo "✅ Daemon running (PID: $$(pgrep -f 'target/debug/daemon'))"

stop-daemon:
	@echo "🛑 Stopping daemon..."
	@pkill -9 -f "target/debug/daemon" || true
	@pkill -9 -f "target/release/daemon" || true
	@echo "✅ Daemon stopped"

# CI pipeline
ci: fmt-check clippy test-all
	@echo "🎉 CI pipeline passed!"

# Clean
clean:
	@echo "🧹 Cleaning build artifacts..."
	cargo clean
	rm -f tests/*.tmp
	@echo "✅ Clean complete"

# Documentation
docs:
	@echo "📚 Building documentation..."
	cargo doc --no-deps --open

# Install targets
install: build
	@echo "Installing binaries..."
	sudo cp target/release/dd-procmgrd /usr/local/bin/dd-procmgrd
	sudo cp target/release/dd-procmgr /usr/local/bin/dd-procmgr
	@echo "Installed to /usr/local/bin/{dd-procmgrd,dd-procmgr}"

uninstall:
	@echo "Uninstalling binaries..."
	sudo rm -f /usr/local/bin/dd-procmgrd
	sudo rm -f /usr/local/bin/dd-procmgr
	@echo "Uninstalled"

# Development helpers
dev-setup:
	@echo "🛠️  Setting up development environment..."
	rustup component add rustfmt clippy
	@echo "✅ Development tools installed"

watch:
	@echo "👀 Watching for changes..."
	cargo watch -x build

# Report test coverage (requires cargo-tarpaulin)
coverage:
	@echo "📊 Generating test coverage report..."
	cargo tarpaulin --out Html --output-dir coverage
	@echo "✅ Coverage report: coverage/index.html"

#
# Docker targets (for macOS and cross-platform testing)
#

docker-build:
	@echo "🐳 Building Docker test image..."
	docker build -f Dockerfile.test -t pm-test .
	@echo "✅ Docker image built"

docker-shell: docker-build
	@echo "🐳 Starting interactive shell..."
	docker run -it --rm \
		-v $(PWD):/workspace \
		-w /workspace \
		--cap-add SYS_ADMIN \
		--security-opt apparmor:unconfined \
		pm-test bash

docker-test: docker-build
	@echo "🐳 Running all tests in Docker..."
	docker run --rm \
		-v $(PWD):/workspace \
		-w /workspace \
		--cap-add SYS_ADMIN \
		--security-opt apparmor:unconfined \
		pm-test \
		bash -c "cargo build --release && make test-engine && make test-e2e"

docker-test-e2e: docker-build
	@echo "🐳 Running e2e tests in Docker..."
	docker run --rm \
		-v $(PWD):/workspace \
		-w /workspace \
		--cap-add SYS_ADMIN \
		--security-opt apparmor:unconfined \
		pm-test \
		bash -c "cargo build && make test-e2e"

docker-mock: docker-build
	@echo "🐳 Testing with mock Datadog services..."
	@echo "📝 Setting up mock services..."
	@mkdir -p docker/mock-datadog
	docker run --rm \
		-v $(PWD):/workspace \
		-w /workspace \
		--cap-add SYS_ADMIN \
		--cap-add NET_BIND_SERVICE \
		--security-opt apparmor:unconfined \
		pm-test \
		bash -c "./docker/scripts/test-mock-datadog.sh"

docker-compose-up:
	@echo "🐳 Starting docker-compose services..."
	docker-compose up -d
	@echo "✅ Services started"
	@echo "   - process-manager: docker-compose exec process-manager bash"
	@echo "   - mock-datadog: docker-compose exec mock-datadog bash"

docker-down:
	@echo "🐳 Stopping docker-compose services..."
	docker-compose down
	@echo "✅ Services stopped"

docker-clean: docker-down
	@echo "🐳 Cleaning Docker images and containers..."
	docker rmi pm-test || true
	docker-compose down -v --rmi all || true
	docker-compose -f docker-compose.dev.yml down -v || true
	@echo "✅ Docker cleanup complete"

#
# Manual testing targets (multi-container development)
#

dev-start: docker-build
	@echo "🐳 Starting development environment..."
	docker-compose -f docker-compose.dev.yml up -d
	@echo "✅ Dev environment started!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Terminal 1: make dev-daemon"
	@echo "     Then run: cargo build --release && ./target/release/daemon examples/mock-datadog.yaml"
	@echo ""
	@echo "  2. Terminal 2: make dev-cli"
	@echo "     Then run: ./target/release/cli list"

dev-daemon:
	@echo "🐳 Connecting to daemon container..."
	docker-compose -f docker-compose.dev.yml exec daemon bash

dev-cli:
	@echo "🐳 Connecting to CLI container..."
	docker-compose -f docker-compose.dev.yml exec cli bash

dev-stop:
	@echo "🐳 Stopping development environment..."
	docker-compose -f docker-compose.dev.yml down
	@echo "✅ Dev environment stopped"

