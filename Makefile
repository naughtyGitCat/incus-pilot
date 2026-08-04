.PHONY: all build-web build-go build clean run

BINARY_NAME=incus-pilot

all: build

build-web:
	@echo "📦 Building Vue 3 Frontend..."
	cd web && npm install && npm run build

build-go:
	@echo "🔨 Building Go Single-Binary with Embed..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY_NAME) main.go

build: build-web build-go
	@echo "✅ Build complete: ./$(BINARY_NAME)"

run: build
	./$(BINARY_NAME)

clean:
	rm -rf $(BINARY_NAME) web/dist
