# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Install git for fetching dependencies
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o gphira-mp ./cmd/server

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
