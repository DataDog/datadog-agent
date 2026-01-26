"""
Locust load generator for payment-service

Instrumented with Datadog APM via ddtrace for automatic tracing.
See: https://docs.datadoghq.com/tracing/trace_collection/dd_libraries/python/
"""

import logging
import os
import random
import uuid

# Datadog APM - Direct instrumentation
from ddtrace import patch_all, tracer
from locust import HttpUser, between, events, task

# Configure the tracer with DD Agent connection
tracer.configure(
    hostname=os.getenv("DD_AGENT_HOST", "dd-agent"),
    port=int(os.getenv("DD_TRACE_AGENT_PORT", "8126")),
)

# Patch all supported libraries for automatic instrumentation
patch_all()

# Configure logging
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)

logging.info("Datadog APM instrumentation initialized")


# ============================================================================
# CONFIGURATION - Environment variables
# ============================================================================

# Wait time between requests (seconds)
WAIT_TIME_MIN = float(os.getenv("WAIT_TIME_MIN", "0.5"))
WAIT_TIME_MAX = float(os.getenv("WAIT_TIME_MAX", "2.0"))

# Task weights - higher weight = more frequent execution
# Payment processing should be the primary flow (50 RPS expected)
# Report generation is much less frequent (1 RPS expected)
TASK_WEIGHTS = {
    "health_check": int(os.getenv("WEIGHT_HEALTH_CHECK", "5")),
    "process_payment": int(os.getenv("WEIGHT_PROCESS_PAYMENT", "50")),
    "generate_report": int(os.getenv("WEIGHT_GENERATE_REPORT", "1")),
}

# Sample data for realistic payment generation
CURRENCIES = ["USD", "EUR", "GBP", "JPY", "CAD", "AUD"]
MERCHANT_IDS = [f"merchant_{i:04d}" for i in range(1, 51)]
CARD_PREFIXES = ["4111111111111", "5500000000000", "340000000000"]  # Visa, MC, Amex patterns
CUSTOMER_IDS = [f"customer_{uuid.uuid4().hex[:8]}" for _ in range(100)]

logging.info(f"Load generator config: wait_time={WAIT_TIME_MIN}-{WAIT_TIME_MAX}s")
logging.info(f"Task weights: {TASK_WEIGHTS}")


# ============================================================================
# MAIN USER CLASS - Payment service load testing
# ============================================================================


class PaymentServiceUser(HttpUser):
    """
    Simulated user performing payment operations.

    - Processes payments with fraud detection
    - Occasionally generates compliance reports
    - Tracks transaction IDs for verification
    """

    # Wait time between requests
    wait_time = between(WAIT_TIME_MIN, WAIT_TIME_MAX)

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.dd_tracer = tracer
        # Store transaction IDs for potential verification
        self.transaction_ids = []

    # ========================================================================
    # TASKS - Weighted by decorator value
    # ========================================================================

    @task(TASK_WEIGHTS["health_check"])
    def task_health_check(self):
        """Check API health."""
        with self.dd_tracer.trace("task_health_check"):
            self.client.get("/health")

    @task(TASK_WEIGHTS["process_payment"])
    def task_process_payment(self):
        """
        Process a payment transaction.
        This is the primary flow (POST /pay) with ~50 RPS expected.
        """
        with self.dd_tracer.trace("task_process_payment") as span:
            # Generate realistic payment data
            transaction_id = str(uuid.uuid4())
            amount = round(random.uniform(10.0, 5000.0), 2)
            currency = random.choice(CURRENCIES)
            merchant_id = random.choice(MERCHANT_IDS)
            customer_id = random.choice(CUSTOMER_IDS)

            # Generate card number (last 4 vary, prefix is fixed for test)
            card_prefix = random.choice(CARD_PREFIXES)
            card_number = card_prefix + str(random.randint(1000, 9999))

            payment_data = {
                "transaction_id": transaction_id,
                "amount": amount,
                "currency": currency,
                "card_number": card_number,
                "card_holder": f"Test User {random.randint(1, 1000)}",
                "expiry_date": f"{random.randint(1, 12):02d}/{random.randint(25, 30)}",
                "cvv": str(random.randint(100, 999)),
                "merchant_id": merchant_id,
                "customer_id": customer_id,
                "description": f"Load test payment {random.randint(1, 10000)}",
            }

            span.set_tag("transaction.id", transaction_id)
            span.set_tag("payment.amount", amount)
            span.set_tag("payment.currency", currency)
            span.set_tag("payment.merchant_id", merchant_id)
            span.set_tag("synthetic_request", "true")

            response = self.client.post("/pay", json=payment_data)

            if response.status_code == 200:
                try:
                    data = response.json()
                    if data.get("status") == "approved":
                        self.transaction_ids.append(transaction_id)
                        # Keep list bounded
                        self.transaction_ids = self.transaction_ids[-100:]
                        span.set_tag("payment.status", "approved")
                except Exception:
                    pass
            elif response.status_code == 402:
                # Payment rejected (fraud detection)
                span.set_tag("payment.status", "rejected")
            else:
                span.set_tag("payment.status", "error")

    @task(TASK_WEIGHTS["generate_report"])
    def task_generate_report(self):
        """
        Generate a compliance report.
        This is a less frequent flow (POST /internal/generate-report) with ~1 RPS expected.
        """
        with self.dd_tracer.trace("task_generate_report") as span:
            # Generate report request
            report_types = ["daily_transactions", "fraud_summary", "merchant_activity", "compliance_audit"]
            report_type = random.choice(report_types)

            # Generate date range (last 7-30 days)
            from datetime import datetime, timedelta
            end_date = datetime.now()
            start_date = end_date - timedelta(days=random.randint(7, 30))

            report_data = {
                "report_type": report_type,
                "start_date": start_date.strftime("%Y-%m-%d"),
                "end_date": end_date.strftime("%Y-%m-%d"),
                "merchant_id": random.choice(MERCHANT_IDS) if random.random() > 0.5 else None,
                "format": random.choice(["pdf", "csv", "json"]),
            }

            # Remove None values
            report_data = {k: v for k, v in report_data.items() if v is not None}

            span.set_tag("report.type", report_type)
            span.set_tag("synthetic_request", "true")

            response = self.client.post("/internal/generate-report", json=report_data)

            if response.status_code == 200:
                try:
                    data = response.json()
                    span.set_tag("report.id", data.get("report_id", "unknown"))
                    span.set_tag("report.status", data.get("status", "unknown"))
                except Exception:
                    pass

    def on_start(self):
        """Called when a user starts."""
        with self.dd_tracer.trace("user_session_start") as span:
            session_id = str(uuid.uuid4())
            span.set_tag("session.id", session_id)
            span.set_tag("synthetic_request", "true")
            logging.info(f"User session started: {session_id}")
            # Verify service is healthy
            self.task_health_check()


# ============================================================================
# LOAD TEST LIFECYCLE EVENTS
# ============================================================================


@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    """Called when the load test starts."""
    print("=" * 60)
    print("Payment Service Load Test Started")
    print(f"   Target: {environment.host}")
    print(f"   Wait time: {WAIT_TIME_MIN}-{WAIT_TIME_MAX}s between requests")
    print(f"   Task weights: {TASK_WEIGHTS}")
    print("=" * 60)


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    """Called when the load test stops."""
    print("=" * 60)
    print("Payment Service Load Test Completed")
    print(f"   Total requests: {environment.stats.total.num_requests}")
    print(f"   Total failures: {environment.stats.total.num_failures}")
    if environment.stats.total.num_requests > 0:
        failure_rate = (
            environment.stats.total.num_failures / environment.stats.total.num_requests
        ) * 100
        print(f"   Failure rate: {failure_rate:.2f}%")
    print("=" * 60)
