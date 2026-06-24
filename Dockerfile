# Stage 1 — build
# Use the full Go image to compile the binary.
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Download dependencies first — this layer is cached as long as
# go.mod and go.sum don't change, so rebuilds after code-only changes
# skip the download entirely.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically linked binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o hekima ./cmd/hekima

# Stage 2 — run
# Minimal Alpine image with only what Hekima needs at runtime.
FROM alpine:latest

# poppler-utils provides pdftotext, required for PDF text extraction.
# ca-certificates allows outbound TLS if ever needed.
RUN apk add --no-cache poppler-utils ca-certificates

# Run as a non-root user. Handling sensitive legal and financial
# documents as root inside a container is not acceptable.
RUN addgroup -S hekima && adduser -S hekima -G hekima

WORKDIR /app
COPY --from=builder /build/hekima .

# Ensure the binary is owned by the hekima user.
RUN chown hekima:hekima /app/hekima

USER hekima

# PORT is the listen port. Override with -e PORT=9090 at runtime.
ENV PORT=8080
EXPOSE 8080

CMD ["sh", "-c", "./hekima --serve --port ${PORT}"]
