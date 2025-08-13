BINARY := bin/micro-api
BINARY_MCP := bin/micro-api-mcp

.PHONY: build run tidy test clean

build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/micro-api
	go build -o $(BINARY_MCP) ./cmd/micro-api-mcp

run: build
	./$(BINARY)

tidy:
	go mod tidy

test:
	go test ./...

clean:
	rm -rf bin

css:
	npx tailwindcss -i ./web/static/input.css -o ./web/static/style.css
