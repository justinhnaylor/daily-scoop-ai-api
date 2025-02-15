FROM golang:1.23.3

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

# Debug: Show Go version and environment - CHECK THIS OUTPUT IN YOUR BUILD LOGS!
RUN go version && \
    go env && \
    echo "GOPATH: $GOPATH" && \
    echo "GOROOT: $GOROOT"

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Show module files for debugging
RUN echo "Module files:" && \
    ls -la && \
    echo "\ngo.mod contents:" && \
    cat go.mod && \
    echo "\ngo.sum contents:" && \
    cat go.sum

# Try to tidy and download with explicit go path
RUN echo "--- Running go mod tidy with explicit path ---" && go mod tidy
RUN echo "--- Running go mod download with explicit path ---" && \
    GOSUMDB=off GOPROXY=https://proxy.golang.org,direct /usr/local/go/bin/go mod download -v || (echo "Download failed with status: $?" && exit 1)

# Copy the rest of the code
COPY . .

# Install Playwright
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# Build the app
RUN go build -v -o app

CMD ["./app"]