# syntax=docker/dockerfile:1

FROM golang:1.18-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/server ./main.go

FROM alpine:3.18
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/bin/server /app/bin/server
COPY --from=builder /app/static /app/static

EXPOSE 9876

CMD ["/app/bin/server"]
