#!/bin/bash
set -e

echo "Building static-app component..."

# Pre-build: Format JavaScript/CSS code
echo "Formatting code..."
if command -v npx &> /dev/null; then
    # Format JavaScript files with Prettier if available
    if [ -f "package.json" ] && grep -q "prettier" package.json 2>/dev/null; then
        npx prettier --write "**/*.{js,jsx,ts,tsx,css,scss,json,html}" 2>/dev/null || true
    fi
    # Run ESLint fix if available
    if [ -f "package.json" ] && grep -q "eslint" package.json 2>/dev/null; then
        npx eslint --fix "**/*.{js,jsx,ts,tsx}" 2>/dev/null || true
    fi
fi

# Build Docker image
echo "Building Docker image..."
docker build -t test-static-app .

echo "✓ Build successful"





