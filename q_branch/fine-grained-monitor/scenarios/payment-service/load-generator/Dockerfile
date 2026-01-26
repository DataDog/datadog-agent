# Load Generator using Locust
FROM python:3.11-slim

LABEL maintainer="vibecoder"
LABEL description="Sophisticated load generator with phase-based experiments and ON/OFF burstiness"

# Install curl, Go (for tests), and build dependencies for psutil (needed for aarch64 builds)
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    golang \
    git \
    gcc \
    g++ \
    python3-dev \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Upgrade pip to ensure it can find binary wheels
RUN pip install --upgrade pip setuptools wheel

# Set working directory
WORKDIR /app

# Copy requirements and install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy load generator files
COPY locustfile.py .
COPY people.json .

# Copy stats-test directory for testing
COPY stats-test ./stats-test

COPY test.sh ./test.sh
RUN chmod +x ./test.sh

# Expose Locust web UI port
EXPOSE 8089

# Set environment variables
ENV LOCUST_HOST=http://todo-backend:8081
ENV LOCUST_USERS=10
ENV LOCUST_SPAWN_RATE=2
ENV LOCUST_RUN_TIME=0
ENV LOCUST_AUTOSTART=true
ENV DD_SERVICE=load-generator
ENV DD_ENV=development
ENV DD_VERSION=1.0.0


# Make sure locustfile.py is compilable
RUN python3 -m py_compile locustfile.py

# Run Locust
# By default, start in headless mode with configurable users
# Can be overridden via docker-compose environment variables
ENTRYPOINT ["ddtrace-run", "locust", "-f", "locustfile.py"]