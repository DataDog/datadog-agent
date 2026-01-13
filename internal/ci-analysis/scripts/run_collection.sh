#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "🚀 CI Data Collection - Containerized Execution"
echo "================================================"
echo ""

# Check if .env file exists
if [ ! -f .env ]; then
    echo "❌ Error: .env file not found"
    echo ""
    echo "Please create .env file with your credentials:"
    echo "  cp .env.example .env"
    echo "  # Edit .env with your actual credentials"
    echo ""
    exit 1
fi

# Load environment variables
source .env

# Validate required variables
MISSING_VARS=()

if [ -z "$GITLAB_TOKEN" ] || [ "$GITLAB_TOKEN" = "your_gitlab_token_here" ]; then
    MISSING_VARS+=("GITLAB_TOKEN")
fi

if [ -z "$DD_API_KEY" ] || [ "$DD_API_KEY" = "your_datadog_api_key_here" ]; then
    MISSING_VARS+=("DD_API_KEY")
fi

if [ -z "$DD_APP_KEY" ] || [ "$DD_APP_KEY" = "your_datadog_app_key_here" ]; then
    MISSING_VARS+=("DD_APP_KEY")
fi

if [ ${#MISSING_VARS[@]} -gt 0 ]; then
    echo "❌ Error: Missing or invalid credentials in .env:"
    for var in "${MISSING_VARS[@]}"; do
        echo "  - $var"
    done
    echo ""
    echo "Please update .env with valid credentials:"
    echo "  - GitLab token: https://gitlab.ddbuild.io/-/profile/personal_access_tokens"
    echo "  - Datadog keys: https://app.datadoghq.com/organization-settings/api-keys"
    echo ""
    exit 1
fi

echo "✅ Credentials validated"
echo ""

# Create output directory
mkdir -p ci_data

# Build Docker image
echo "🐳 Building Docker image..."
docker build -t ci-analysis:latest . --quiet

# Show what we're about to run
echo ""
echo "📊 Data Collection Parameters:"
echo "  - GitLab URL: ${GITLAB_URL:-https://gitlab.ddbuild.io}"
echo "  - Project ID: ${GITLAB_PROJECT_ID:-14}"
echo "  - Days: ${DAYS:-180}"
echo "  - Max Pipelines: ${MAX_PIPELINES:-1000}"
echo "  - Datadog Site: ${DD_SITE:-datadoghq.com}"
echo "  - Output: $(pwd)/ci_data/"
echo ""

# Ask for confirmation
read -p "Run data collection? [y/N] " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ Aborted"
    exit 0
fi

# Run GitLab extraction
echo ""
echo "📊 Running GitLab API extraction..."
echo "================================================"
docker run --rm \
    -e GITLAB_TOKEN \
    -e GITLAB_URL \
    -v "$(pwd)/ci_data:/app/ci_data" \
    ci-analysis:latest \
    python gitlab_api_extraction.py \
    --project-id "${GITLAB_PROJECT_ID:-14}" \
    --days "${DAYS:-180}" \
    --max-pipelines "${MAX_PIPELINES:-1000}" \
    --output-dir /app/ci_data

echo ""
echo "✅ GitLab extraction complete"

# Run Datadog extraction
echo ""
echo "📊 Running Datadog CI Visibility extraction..."
echo "================================================"
docker run --rm \
    -e DD_API_KEY \
    -e DD_APP_KEY \
    -e DD_SITE \
    -v "$(pwd)/ci_data:/app/ci_data" \
    ci-analysis:latest \
    python datadog_ci_visibility_queries.py \
    --days "${DAYS:-180}" \
    --output-dir /app/ci_data

echo ""
echo "✅ Datadog extraction complete"

# Show results
echo ""
echo "================================================"
echo "✅ Data collection complete!"
echo ""
echo "📁 Output files in: $(pwd)/ci_data/"
ls -lh ci_data/*.csv 2>/dev/null || echo "  (No CSV files generated - check logs above)"
echo ""
echo "Next steps:"
echo "  1. Review collected data: ls -lh ci_data/"
echo "  2. Validate data quality"
echo "  3. Run analysis (see README.md)"
echo ""
