FROM python:3.11-slim

# Install system dependencies (ss command from iproute2)
RUN apt-get update && apt-get install -y --no-install-recommends \
    iproute2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY src/ src/

# Copy scripts
COPY scripts/ scripts/

# Create models directory
RUN mkdir -p models/healthy_baseline

# Default command
CMD ["python", "scripts/collect_baseline.py", "--samples", "60", "--interval", "10"]
