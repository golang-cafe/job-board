#!/usr/bin/env bash

export NEWSLETTER_JOBS_TO_SEND=5
export TWITTER_ACCESS_TOKEN=a123
export TWITTER_ACCESS_TOKEN_SECRET=a123
export TWITTER_CLIENT_KEY=a123
export TWITTER_CLIENT_SECRET=a123
export TWITTER_JOBS_TO_POST=5
export STRIPE_KEY=k_123
export STRIPE_ENDPOINT_SECRET=wc_123
export STRIPE_PUBLISHABLE_KEY=pk_123
export EMAIL_API_KEY=a123
export PORT=9876
export JWT_SIGNING_KEY=a123
export IP_GEOLOCATION_CURRENCY_MAPPING_FILE=static/country2currency.csv
export IP_GEOLOCATION_GEOLITE_DB_FILE=static/geolite2.mmdb
export SESSION_KEY=a123
export ADMIN_EMAIL=x@example.com
export SUPPORT_EMAIL=support@example.com
export MAILERLITE_API_KEY=a123
export SENTRY_DSN=https://localhost:123
export ENV=dev
export HEROKU_POSTGRESQL_PINK_URL="postgresql://postgres:passw0rd!@localhost:5432/postgres?sslmode=disable"
export GO111MODULE=on
export CLOUDFLARE_API_TOKEN=a123
export CLOUDFLARE_ZONE_TAG=a123
export CLOUDFLARE_API_ENDPOINT=https://api.cloudflare.com/client/v4/graphql
export MACHINE_TOKEN=a123
export TELEGRAM_API_TOKEN=a123
export TELEGRAM_CHANNEL_ID=1233123123
export FX_API_KEY=a123
export SITE_NAME="Golang Cafe"
export SITE_JOB_CATEGORY="golang"
export SITE_HOST="golang.cafe"
export SITE_GITHUB="golang-cafe/golang.cafe"
export SITE_TWITTER="golangcafe"
export SITE_YOUTUBE="golangcafe"
export SITE_LINKEDIN="golangcafe"
export SITE_TELEGRAM="golangcafe"
export SITE_LOGO_IMAGE_ID="2DUDLDHdnx04GK8C45o5d8jVZ0v"
export EMAIL2_API_KEY="i"
export NO_REPLY_EMAIL="no-reply@golang.cafe"
export PRIMARY_COLOR="#25a79b"
export SECONDARY_COLOR="#43D7C9"

go build -o bin/server cmd/server/main.go

./bin/server
