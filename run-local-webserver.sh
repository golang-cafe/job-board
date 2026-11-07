#!/usr/bin/env bash
set -euo pipefail

if ! command -v docker &> /dev/null
then
	echo 'Docker is required for this script. Please install it at https://docs.docker.com/get-docker/'
	exit 1
fi

ENV_FILE="${ENV_FILE:-local.env}"
if [ ! -f "${ENV_FILE}" ] && [ -f "local.env.example" ]; then
	echo "No ${ENV_FILE} found, copying defaults from local.env.example..."
	cp local.env.example "${ENV_FILE}"
fi

docker build -f Dockerfile.local -t golang-cafe-job-board-local .

docker run -it --rm \
	-w /app \
	-v "$(pwd):/app" \
	-p 9876:9876 \
	--add-host=host.docker.internal:host-gateway \
	--env-file "${ENV_FILE}" \
	golang-cafe-job-board-local
