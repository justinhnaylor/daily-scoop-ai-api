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

# Copy go.mod and go.sum first
COPY go.mod go.sum ./

# More verbose module download
RUN go mod download -x || (echo "Module download failed" && exit 1)

# Copy the rest of the code
COPY . .

# Install Playwright with verbose output
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# Build the app with verbose output
RUN go build -v -o app

CMD ["./app"] 