FROM node:18-slim

WORKDIR /app

# Provide a glibc-based Chromium for Puppeteer-driven integration tests
ENV PUPPETEER_SKIP_DOWNLOAD=true
ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true
ENV PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium

# Install runtime deps and Chromium
RUN apt-get update && apt-get install -y --no-install-recommends \
        bash \
        chromium \
        ca-certificates \
        fonts-liberation \
        libatk-bridge2.0-0 \
        libatk1.0-0 \
        libnss3 \
        libxss1 \
        libasound2 \
        libxshmfence1 \
        libgbm1 \
        libgtk-3-0 \
        wget \
        curl \
    && rm -rf /var/lib/apt/lists/*


# Copy package files
COPY package*.json ./

RUN npm install --omit=dev 

# Copy application files
COPY . .

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=10s \
  CMD wget --quiet --tries=1 -O /dev/null http://localhost:8080/health || exit 1

CMD ["npm", "start"]

