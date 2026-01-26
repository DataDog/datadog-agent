#!/bin/sh
set -e

COMPONENT_NAME="${COMPONENT_NAME:-payment-service}"

is_inside_container() {
    [ "${RUNNING_IN_CONTAINER:-}" = "true" ]
}

run_tests() {
    echo "Testing payment-service component..."

    PORT="${PORT:-8081}"
    HOST="${HOST:-localhost}"

    echo "• Checking health endpoint..."
    HEALTH_RESPONSE=$(curl -sf "http://${HOST}:${PORT}/health")
    echo "  Health response: ${HEALTH_RESPONSE}"

    echo "• Listing swagger availability..."
    curl -sf "http://${HOST}:${PORT}/swagger/doc.json" >/dev/null
    echo "  Swagger documentation available"

    echo "• Testing POST /pay endpoint..."
    PAY_RESPONSE=$(curl -sf -X POST "http://${HOST}:${PORT}/pay" \
        -H "Content-Type: application/json" \
        -d '{
            "amount": 99.99,
            "currency": "USD",
            "card_number": "4111111111111234",
            "card_holder": "Test User",
            "expiry_date": "12/25",
            "cvv": "123",
            "merchant_id": "merchant_0001",
            "description": "Smoke test payment"
        }')
    echo "  Payment response: ${PAY_RESPONSE}"

    # Verify response contains expected fields
    echo "${PAY_RESPONSE}" | grep -q "transaction_id" || { echo "ERROR: Missing transaction_id"; exit 1; }
    echo "${PAY_RESPONSE}" | grep -q "status" || { echo "ERROR: Missing status"; exit 1; }
    echo "  Payment endpoint working correctly"

    echo "• Testing POST /internal/generate-report endpoint..."
    REPORT_RESPONSE=$(curl -sf -X POST "http://${HOST}:${PORT}/internal/generate-report" \
        -H "Content-Type: application/json" \
        -d '{
            "report_type": "daily_transactions",
            "start_date": "2024-01-01",
            "end_date": "2024-01-31",
            "format": "pdf"
        }')
    echo "  Report response: ${REPORT_RESPONSE}"

    # Verify response contains expected fields
    echo "${REPORT_RESPONSE}" | grep -q "report_id" || { echo "ERROR: Missing report_id"; exit 1; }
    echo "${REPORT_RESPONSE}" | grep -q "status" || { echo "ERROR: Missing status"; exit 1; }
    echo "  Report generation endpoint working correctly"

    echo "• Testing validation error handling..."
    VALIDATION_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://${HOST}:${PORT}/pay" \
        -H "Content-Type: application/json" \
        -d '{"invalid": "data"}')
    if [ "${VALIDATION_RESPONSE}" = "400" ]; then
        echo "  Validation errors handled correctly (400 response)"
    else
        echo "  WARNING: Validation error returned unexpected status: ${VALIDATION_RESPONSE}"
    fi

    echo "✓ ALL TESTS PASSED"
}

if is_inside_container; then
    run_tests
else
    echo "Running tests inside container '${COMPONENT_NAME}'..."
    SCRIPT_CONTENT=$(cat "$0")
    docker compose exec -T "${COMPONENT_NAME}" sh -c "$SCRIPT_CONTENT"
fi
