BINARY      := dashboard
IMAGE       := ghcr.io/imodyhk/homelab-dashboard
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
GOFLAGS     := -trimpath
PLATFORMS   ?= linux/amd64

.PHONY: dev build web image test lint clean tidy

dev:
	@echo "Backend on :8080, frontend on :5173 with /api proxied. Export the variables from deploy/.env.example first."
	@( cd web && npm run dev ) & \
	ALLOWED_ORIGINS=localhost:5173 SECURE_COOKIES=false LISTEN_ADDR=127.0.0.1:8080 CGO_ENABLED=0 go run ./cmd/dashboard

web:
	cd web && npm ci --no-audit --no-fund && npm run build

build: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/dashboard
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags "-s -w" -o bin/smart ./cmd/smart

image:
	docker buildx build --platform $(PLATFORMS) --target dashboard --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest --load .
	docker buildx build --platform $(PLATFORMS) --target smart -t $(IMAGE)-smart:$(VERSION) -t $(IMAGE)-smart:latest --load .

test:
	CGO_ENABLED=0 go test -race -count=1 ./...

lint:
	go vet ./...
	staticcheck ./...
	cd web && npx tsc -b

tidy:
	go mod tidy

clean:
	rm -rf bin web/dist/assets web/dist/index.html web/dist/favicon.svg
