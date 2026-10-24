# Multi-stage build for production deployment
FROM golang:1.18-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy application source and assets
COPY . .

# Compile static standalone binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server .

# Production runtime stage
FROM alpine:3.18

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server /app/server
COPY --from=builder /app/static /app/static

ARG PORT=9876
ENV PORT=${PORT}
EXPOSE ${PORT}

ENTRYPOINT ["/app/server"]
