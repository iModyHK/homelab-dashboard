FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/dashboard ./cmd/dashboard \
 && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w" -o /out/smart ./cmd/smart

FROM gcr.io/distroless/static-debian12:nonroot AS dashboard
COPY --from=build /out/dashboard /dashboard
ENV DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/dashboard", "healthcheck"]
ENTRYPOINT ["/dashboard"]

FROM alpine:3.20 AS smart
RUN apk add --no-cache smartmontools
COPY --from=build /out/smart /smart
EXPOSE 9633
HEALTHCHECK --interval=60s --timeout=5s --start-period=90s --retries=3 CMD ["/smart", "healthcheck"]
ENTRYPOINT ["/smart"]
