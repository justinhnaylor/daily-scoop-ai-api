#!/bin/bash
export TZ="America/New_York"
echo "Running at $(date)"


cd /Users/justinnaylor/projects/daily-scoop-ai-api
go run . -mode=daily