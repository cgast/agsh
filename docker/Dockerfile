# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /agsh ./cmd/agsh

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache git bash ca-certificates
COPY --from=builder /agsh /usr/local/bin/agsh

# Sandbox workspace
RUN mkdir -p /workspace /data
WORKDIR /workspace

# Non-root user for sandbox
RUN adduser -D -h /workspace agsh
USER agsh

ENTRYPOINT ["agsh"]
