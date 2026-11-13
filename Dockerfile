# Production multi-stage Dockerfile for DigitalOcean App Platform and container deployments

FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

# Cache dependency downloads separately from source changes
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/job-board .

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup \
    && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /out/job-board /app/job-board

# Optional filesystem static assets directory referenced by /s/ route
RUN mkdir -p /app/static/assets && chown -R appuser:appgroup /app

USER appuser

# Runtime PORT is supplied via environment variables (e.g. DigitalOcean App Platform sets PORT=8080)
EXPOSE 8080

ENTRYPOINT ["/app/job-board"]
