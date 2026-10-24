#!/usr/bin/env bash
set -e

# Ensure Docker is installed and running
if ! command -v docker &> /dev/null; then
    echo "Error: docker is required to run the local webserver without host Go dependencies." >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Ensure local.env exists with baseline development configuration
if [ ! -f "local.env" ]; then
    echo "Creating default local.env file..."
    cat << 'EOF' > local.env
PORT=9876
DATABASE_URL=postgres://postgres:postgres@localhost:5432/jobboard?sslmode=disable
SESSION_KEY=development-session-key-32-chars-long-1234
MACHINE_TOKEN=local-machine-token-for-cron-testing
APP_ENV=local
EOF
fi

IMAGE_NAME="job-board-local"

echo "Building local development container ($IMAGE_NAME)..."
docker build -f Dockerfile.local -t "$IMAGE_NAME" .

echo "Starting job-board in development mode on http://localhost:9876 (with hot reload)..."
exec docker run -it --rm \
    -w /app \
    -v "$PWD":/app \
    -p 9876:9876 \
    --env-file local.env \
    "$IMAGE_NAME"
