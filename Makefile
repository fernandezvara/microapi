BINARY := bin/micro-api

.PHONY: build run tidy test clean

build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/micro-api

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
