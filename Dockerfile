FROM golang:1.23.3

# Add build argument for mode
ARG MODE=daily

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
    pkg-config \
    # Add these new dependencies
    libxcursor1 \
    libgtk-3-0 \
    libgdk-pixbuf2.0-0 \
    # Additional dependencies that might be needed
    libx11-xcb1 \
    libxcb1 \
    libxss1 \
    libxtst6 \
    libnss3-dev \
    libpango-1.0-0 \
    libcairo2 \
    fonts-liberation \
    xvfb \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod files first
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with verbose output
RUN go build -v -o app && \
    chmod +x app && \
    ls -la app

# Install Playwright
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# Use the build argument in the command
ENTRYPOINT ["/app/app"]
CMD ["-mode=${MODE}"]