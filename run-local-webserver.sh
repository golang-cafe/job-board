#!/usr/bin/env bash

docker build -f Dockerfile.local -t golang-cafe-local .

docker run -it --rm \
    -w /app \
    -v "$(pwd):/app" \
    --add-host=host.docker.internal:host-gateway \
    -p 9876:9876 \
    --env-file local.env \
    golang-cafe-local
