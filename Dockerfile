# -- Stage 1 -- #
# Compile the app.
FROM golang:1.16-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o bin/server cmd/server/main.go

# -- Stage 2 -- #
# Create the final environment with the compiled binary.
FROM alpine
EXPOSE 9876
COPY --from=builder /app/bin/server .
COPY --from=builder /app/static ./static/
CMD ["./server"]