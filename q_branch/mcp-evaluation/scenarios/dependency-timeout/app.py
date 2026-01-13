#!/usr/bin/env python3
"""Order processing application."""
import time
import urllib.request
import urllib.error
from datetime import datetime

PAYMENT_SERVICE = "http://localhost:8080"
REQUEST_TIMEOUT = 2
MAX_ATTEMPTS = 5

def log(msg):
    timestamp = datetime.now().strftime("%H:%M:%S")
    print(f"[{timestamp}] {msg}", flush=True)

log("Order processor starting...")
log(f"Payment service: {PAYMENT_SERVICE}")

# Store pending orders for processing
pending_orders = []
order_id = 0

while True:
    order_id += 1
    start = time.time()
    success = False

    for attempt in range(1, MAX_ATTEMPTS + 1):
        try:
            response = urllib.request.urlopen(PAYMENT_SERVICE, timeout=REQUEST_TIMEOUT)
            result = response.read().decode()
            elapsed_ms = (time.time() - start) * 1000
            log(f"Order {order_id} completed in {elapsed_ms:.0f}ms")
            success = True
            break
        except urllib.error.URLError as e:
            if attempt < MAX_ATTEMPTS:
                log(f"Order {order_id} payment error (attempt {attempt}/{MAX_ATTEMPTS})")
                # Keep pending order in memory for retry
                pending_orders.append({
                    'order_id': order_id,
                    'attempt': attempt,
                    'error': str(e),
                    'timestamp': time.time(),
                    'data': 'x' * 1024  # Order data
                })
            else:
                elapsed_ms = (time.time() - start) * 1000
                log(f"Order {order_id} payment failed after {MAX_ATTEMPTS} attempts ({elapsed_ms:.0f}ms)")
                # Keep failed order in memory
                pending_orders.append({
                    'order_id': order_id,
                    'attempts': MAX_ATTEMPTS,
                    'error': str(e),
                    'timestamp': time.time(),
                    'data': 'x' * 5120  # Extended order data
                })

    # Show pending queue size
    pending_mb = (len(pending_orders) * 6) / 1024
    if order_id % 10 == 0:
        log(f"Pending orders: {len(pending_orders)} (~{pending_mb:.1f}MB in memory)")

    # Brief delay between orders
    time.sleep(0.5)
