#!/usr/bin/env bash

docker run -it --rm \
    -w /app \
    -v $(pwd):/app \
    -p 9876:9876 \
    --env-file local.env \
    cosmtrek/air \
    --build.cmd "go build -o bin/server cmd/server/main.go" --build.bin "./bin/server"
