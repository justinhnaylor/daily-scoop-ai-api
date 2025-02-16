FROM golang:1.23.3

# Add build argument for mode
ARG MODE=daily

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
    libasound2 \
    libvips-dev \
    pkg-config \
    libxcursor1 \
    libgtk-3-0 \
    libgdk-pixbuf2.0-0 \
    libx11-xcb1 \
    libxcb1 \
    libxss1 \
    libxtst6 \
    libnss3-dev \
    libpango-1.0-0 \
    libcairo2 \
    fonts-liberation \
    xvfb \
    python3-dev \
    python3-pip \
    python3-venv \
    python3-virtualenv \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean autoclean \
    && apt-get autoremove -y

WORKDIR /app

# Copy go mod files first
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Set up Python virtual environment and install dependencies
RUN python3 -m venv --clear .venv && \
    . .venv/bin/activate && \
    /app/.venv/bin/pip install --no-cache-dir wheel && \
    /app/.venv/bin/pip install --no-cache-dir \
        beautifulsoup4 \
        requests \
        newspaper3k \
        nltk \
        openai \
        google-cloud-texttospeech \
        transformers \
        torch \
        google-generativeai>=0.8.0 && \
    # Clean up cache and temporary files
    rm -rf /root/.cache/pip && \
    find /usr/local -type f -name '*.pyc' -delete && \
    find /usr/local -type d -name '__pycache__' -delete

# Download NLTK data
RUN /app/.venv/bin/python -m nltk.downloader punkt

# After setting up Python environment and before building the application
# Create Python scripts directory
RUN mkdir -p /tmp/summarizer

# Copy Python scripts
COPY *.py /tmp/summarizer/

# Set permissions
RUN chmod -R 755 /tmp/summarizer

# Set environment variable for script location
ENV SUMMARIZER_SCRIPT_DIR=/tmp/summarizer

# Build the application
RUN go build -v -o app && chmod +x app

# Install Playwright
RUN go build -v -o /usr/local/bin/playwright github.com/playwright-community/playwright-go/cmd/playwright
RUN playwright install --with-deps chromium

# Environment variables
ENV DISPLAY=:99
ENV PLAYWRIGHT_BROWSERS_PATH=/root/.cache/ms-playwright
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
ENV PATH="/app/.venv/bin:${PATH}"
ENV PYTHONPATH="/app/.venv/lib/python3/site-packages"

# Create a startup script
RUN echo '#!/bin/sh\n\
. /app/.venv/bin/activate\n\
export PATH="/app/.venv/bin:${PATH}"\n\
export PYTHONPATH="/app/.venv/lib/python3/site-packages"\n\
Xvfb :99 -screen 0 1280x1024x24 &\n\
sleep 1\n\
/app/app -mode=${MODE:-daily}' > /start.sh && \
chmod +x /start.sh

ENTRYPOINT ["/bin/sh"]
CMD ["/start.sh"]