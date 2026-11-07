# Stage 1: Build the static Go binary with embedded static assets
FROM golang:1.16-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/server ./main.go

# Stage 2: Minimal production runtime image for DigitalOcean App Platform
FROM alpine:3.18

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bin/server /app/server

EXPOSE 9876

CMD ["/app/server"]
