FROM python:3.11-slim

WORKDIR /app

# Install dependencies
RUN pip install --no-cache-dir \
    datadog-api-client \
    python-gitlab \
    pandas \
    requests

# Copy scripts
COPY *.py /app/
COPY *.md /app/

# Create output directory
RUN mkdir -p /app/ci_data

# Default command shows help
CMD ["python", "gitlab_api_extraction.py", "--help"]
