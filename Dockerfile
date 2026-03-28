# ─── Stage 1: build ───────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Copy module files first for layer caching
COPY go.mod ./
RUN go mod download

# Copy source
COPY . .

# Build the CLI
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /daneel ./cmd/daneel

# ─── Stage 2: runtime ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /daneel /daneel

# Expose nothing by default; connectors bind their own ports
ENTRYPOINT ["/daneel"]
CMD ["--help"]
