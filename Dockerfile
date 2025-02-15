FROM golang:1.21.6

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

# Copy entire project first to resolve dependencies
COPY . .

# Debug: Show Go version and environment
RUN go version && \
    go env && \
    echo "GOPATH: $GOPATH" && \
    echo "GOROOT: $GOROOT"

# Try to tidy and download modules
RUN go mod tidy && \
    GOSUMDB=off GOPROXY=https://proxy.golang.org,direct go mod download -v

# Install Playwright
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# Build the app
RUN go build -v -o app

CMD ["./app"] 