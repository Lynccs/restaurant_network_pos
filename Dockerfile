# ── Stage 1: build ───────────────────────────────────────────────────────────
FROM golang:1.25 AS builder

WORKDIR /app

# Install the templ code-generation tool (version must match go.mod)
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001

# Download dependencies first so this layer is cached
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree
COPY . .

# Generate *_templ.go files from *.templ sources
RUN templ generate

# Build a static binary (CGO_ENABLED=0 → no libc dependency at runtime)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /restaurant_pos ./cmd/server

# ── Stage 2: runtime ─────────────────────────────────────────────────────────
FROM debian:bookworm-slim

# ca-certificates are required for TLS connections (e.g. to SQL Server over TLS)
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the compiled binary
COPY --from=builder /restaurant_pos .

# Copy static assets served directly by the Go HTTP server
COPY --from=builder /app/static ./static

# Copy runtime data (supplier/ingredient seed mappings)
COPY --from=builder /app/data ./data

EXPOSE 8080

CMD ["./restaurant_pos"]
