BINARY      := dashboard
IMAGE       := ghcr.io/imodyhk/homelab-dashboard
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
GOFLAGS     := -trimpath
PLATFORMS   ?= linux/amd64

.PHONY: dev build web image test lint clean tidy

dev:
	@echo "Backend on :8080, frontend on :5173 with /api proxied"
	@( cd web && npm run dev ) & \
	CGO_ENABLED=0 go run ./cmd/dashboard

web:
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/dashboard

image:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest --load .

test:
	CGO_ENABLED=0 go test -race -count=1 ./...

lint:
	go vet ./...
	staticcheck ./...
	cd web && npx tsc -b --noEmit

tidy:
	go mod tidy

clean:
	rm -rf bin web/dist
