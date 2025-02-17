#!/bin/bash
MODE=$1
if [ -z "$MODE" ]; then
    MODE="daily"
fi

# Run docker-compose with auto-removal
docker-compose up --build --abort-on-container-exit --exit-code-from app

# Clean up regardless of exit status
docker-compose down 