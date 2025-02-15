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

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the code
COPY . .

# Install Playwright
RUN go build -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

RUN go build -o app

CMD ["./app"] 