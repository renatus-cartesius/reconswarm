FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN go build -o /app/bin/reconswarm ./main.go

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
# ca-certificates for HTTPS, openssh-client for SSH control
RUN apk add --no-cache ca-certificates openssh-client

# Copy binary from builder
COPY --from=builder /app/bin/reconswarm /app/reconswarm

# Create directory for SSH keys
RUN mkdir -p /tmp/reconswarm

# Expose gRPC port
EXPOSE 50051

# Run the server
CMD ["/app/reconswarm", "server"]
