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

# Debug: List contents before copy
RUN pwd && ls -la

# Copy entire project first
COPY . .

# Debug: List contents after copy
RUN pwd && ls -la

# Initialize module if needed
RUN if [ ! -f go.mod ]; then go mod init main; fi

# Download dependencies with debug output
RUN go mod tidy -v

# Install Playwright
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# Build the app
RUN go build -v -o app

CMD ["./app"] 