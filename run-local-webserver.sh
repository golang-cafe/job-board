#!/usr/bin/env bash
set -euo pipefail

docker build -f Dockerfile.local -t golang-cafe-local .

docker run -it --rm \
    -w /app \
    -v "$(pwd):/app" \
    -p 9876:9876 \
    --env-file local.env \
    golang-cafe-local
