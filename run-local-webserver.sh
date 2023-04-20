#!/usr/bin/env bash

docker run -it --rm \
    -v $(pwd):/app \
    -w /app/cmd/server \
    -e HOST=0.0.0.0 \
    -e PORT=9876 \
    --env-file local.env \
    cosmtrek/air \
    -c /app/.air.toml