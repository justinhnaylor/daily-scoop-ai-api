#!/bin/bash

# Set the timezone
export TZ="America/New_York"

# Add the Go binary location to the PATH (if it's not already in PATH)
export PATH=$PATH:/usr/local/go/bin

# Log the running time
echo "Running at $(date)"

# Navigate to the project directory
cd /Users/justinnaylor/projects/daily-scoop-ai-api

# Run the Go command
go run . -mode=daily
