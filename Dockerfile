FROM golang:1.21

# Install system dependencies
RUN apt-get update && apt-get install -y \
    libnss3 \
    libnspr4 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libxkbcommon0 \
    libxcomposite1 \
    libxdamage1 \
    libxfixes3 \
    libxrandr2 \
    libgbm1 \
    libasound2

WORKDIR /app

# Copy entire project first
COPY . .

# Debug: Show directory contents and go.mod content
RUN echo "Current directory:" && \
    pwd && \
    echo "\nDirectory contents:" && \
    ls -la && \
    echo "\ngo.mod contents:" && \
    cat go.mod || echo "go.mod not found"

# Try to initialize and tidy with error output
RUN go mod init main 2>&1 || true && \
    go mod tidy -v 2>&1 && \
    echo "go.mod after tidy:" && \
    cat go.mod

# Install Playwright
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# Build the app
RUN go build -v -o app

CMD ["./app"] 