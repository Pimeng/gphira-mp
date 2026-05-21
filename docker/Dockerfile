# Build stage - 使用构建平台原生架构，通过 Go 交叉编译避免 QEMU 模拟
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /build

# Install git for fetching dependencies
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS TARGETARCH
ARG PHIRA_MP_VERSION
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X github.com/Pimeng/gphira-mp-next/internal/version.version=${PHIRA_MP_VERSION}" -o gphira-mp ./cmd/server

# Runtime stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/gphira-mp /app/gphira-mp
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Create directories for data
RUN mkdir -p /app/data/cache /app/logs /app/record

EXPOSE 12346
EXPOSE 12347

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/app/gphira-mp", "--config", "/app/config.yaml"]
