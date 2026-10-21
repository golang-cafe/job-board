# -- Stage 1 -- #
# Compile the app.
FROM golang:1.18-alpine AS builder
ENV CGO_ENABLED=0
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY ./internal ./internal
COPY ./static ./static
RUN go build -o bin/server main.go

# -- Stage 2 -- #
# Create the final environment with the compiled binary.
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
ARG PORT=9876
ENV PORT=${PORT}
EXPOSE ${PORT}
COPY --from=builder /app/bin/server .
COPY --from=builder /app/static ./static/
CMD ["./server"]
