FROM golang:1.20-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/server main.go

FROM alpine:3.18

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/static /app/static

EXPOSE 8080

CMD ["/app/server"]
