FROM golang:1.23.3

# Install system dependencies including libvips
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
    libasound2 \
    libvips-dev \
    pkg-config

WORKDIR /app

# Copy entire project first to resolve dependencies
COPY . .

# Debug: Show Go version and environment
RUN go version && \
    go env && \
    echo "GOPATH: $GOPATH" && \
    echo "GOROOT: $GOROOT"

# Try to tidy and download modules
RUN echo "--- Running go mod tidy ---" && \
    go mod tidy && \
    echo "--- Running go mod download ---" && \
    GOSUMDB=off GOPROXY=https://proxy.golang.org,direct go mod download

# Install Playwright
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# List all files and try to build with error capture
RUN echo "Contents of current directory:" && \
    ls -la && \
    echo "--- Building app ---" && \
    go build -v 2>&1 || (echo "Build failed. Error output above.")

CMD ["./app"]