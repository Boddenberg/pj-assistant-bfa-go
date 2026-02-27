# ============================================================
# BFA (Go) - Multi-stage build — Railway compatible
# ============================================================
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go.mod first (go.sum may not exist yet)
COPY go.mod ./
RUN go mod download 2>/dev/null || true

# Copy source
COPY . .

# Resolve deps and build
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bfa ./cmd/bfa

# --- Runtime (minimal) ---
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=builder /bfa .

# Railway injects PORT automatically
EXPOSE 8080

ENTRYPOINT ["./bfa"]
