# Build stage - 使用构建平台原生架构，通过 Go 交叉编译避免 QEMU 模拟
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /build

# Install git for fetching dependencies, plus ca-certificates and tzdata for runtime
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS TARGETARCH
ARG PHIRA_MP_VERSION
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X github.com/Pimeng/gphira-mp-next/internal/version.version=${PHIRA_MP_VERSION}" -o gphira-mp ./cmd/server

# Prepare runtime files in builder stage to avoid QEMU in runtime stage
RUN mkdir -p /build/out/app/data/cache /build/out/app/logs /build/out/app/record
COPY docker/entrypoint.sh /build/out/entrypoint.sh

# Runtime stage - 无 RUN 指令，完全避免 QEMU
FROM alpine:3.20

WORKDIR /app

COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /build/out/entrypoint.sh /entrypoint.sh
COPY --from=builder /build/out/app /app
COPY --from=builder /build/gphira-mp /app/gphira-mp

EXPOSE 12346
EXPOSE 12347

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/app/gphira-mp", "--config", "/app/config.yaml"]
