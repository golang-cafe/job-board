#!/usr/bin/env bash

docker build -t golang-cafe-job-board .
docker run -it --rm \
    --network host \
    -p 9876:9876 \
    --env-file local.env \
    golang-cafe-job-board
