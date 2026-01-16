
.PHONY: build
build: clean
	@echo "building mcp server"
	go build -o ./bin/mcp-server cmd/mcp-server/main.go

.PHONY: clean
clean:
	@echo "cleaning bin/ directory"
	@rm bin/* || true
