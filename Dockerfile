# ============================================================
# BFA (Go) - Multi-stage build
# ============================================================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Dependencies first (cached layer)
COPY go.mod go.sum ./
RUN go mod download || true

# Build
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bfa ./cmd/bfa

# --- Runtime ---
FROM alpine:3.20

RUN apk --no-cache add ca-certificates
WORKDIR /app

COPY --from=builder /bfa .

EXPOSE 8080

ENTRYPOINT ["./bfa"]
