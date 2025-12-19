#!/usr/bin/env python3
"""
Self-Induced Feedback Loop - "Healthy But Broken" Pattern

Simulates an application with internal health feedback that K8s can't see.

K8s sees: Running, Readiness probe passing, no events
At 15s: "Averaging 45% CPU, looks stable"
At 1Hz: Characteristic ramp-up, plateau, cliff-drop, recovery pattern
"""

import http.server
import socketserver
import sys
import threading
import time

HEALTH_RECOVERY_RATE = 5.0
HEALTH_DEGRADATION_RATE = 15.0
WORK_HEALTH_THRESHOLD = 30
CRITICAL_HEALTH_THRESHOLD = 15
MAX_HEALTH = 100
HEALTH_CHECK_PORT = 8080


class ApplicationState:
    def __init__(self):
        self.health = MAX_HEALTH
        self.lock = threading.Lock()
        self.work_done = 0
        self.cycles = 0
        self.state = "healthy"

    def get_health(self):
        with self.lock:
            return self.health

    def update_health(self, delta):
        with self.lock:
            self.health = max(0, min(MAX_HEALTH, self.health + delta))
            return self.health

    def get_state(self):
        with self.lock:
            return self.state

    def set_state(self, state):
        with self.lock:
            if self.state != state:
                self.cycles += 1
                print(f"[State] {self.state} -> {state} (cycle {self.cycles})")
                sys.stdout.flush()
            self.state = state


def cpu_work(duration):
    end_time = time.time() + duration
    result = 0
    while time.time() < end_time:
        for i in range(10000):
            result += i * i
    return result


def work_loop(app_state):
    while True:
        health = app_state.get_health()

        if health >= WORK_HEALTH_THRESHOLD:
            app_state.set_state("healthy")
            cpu_work(0.5)
            app_state.update_health(-HEALTH_DEGRADATION_RATE * 0.5)

        elif health >= CRITICAL_HEALTH_THRESHOLD:
            app_state.set_state("degraded")
            cpu_work(0.2)
            app_state.update_health(HEALTH_RECOVERY_RATE * 0.3 - HEALTH_DEGRADATION_RATE * 0.2)
            time.sleep(0.1)

        else:
            app_state.set_state("critical")
            time.sleep(0.5)
            app_state.update_health(HEALTH_RECOVERY_RATE * 0.5)


def monitor(app_state):
    while True:
        time.sleep(5)
        health = app_state.get_health()
        state = app_state.get_state()

        bar_len = int(health / 2)
        health_bar = "#" * bar_len + "-" * (50 - bar_len)

        print(f"[Monitor] Health: {health:5.1f} [{health_bar}] State: {state} Cycles: {app_state.cycles}")
        sys.stdout.flush()


class HealthHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        # Always returns 200 - K8s thinks we're healthy!
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'OK\n')

    def log_message(self, format, *args):
        pass


def health_server():
    with socketserver.TCPServer(("", HEALTH_CHECK_PORT), HealthHandler) as httpd:
        httpd.serve_forever()


def main():
    print("=" * 60)
    print("Self-Induced Feedback Loop Demo")
    print("=" * 60)
    print(f"Thresholds: work={WORK_HEALTH_THRESHOLD}, critical={CRITICAL_HEALTH_THRESHOLD}")
    print(f"Health check: :{HEALTH_CHECK_PORT} (always returns 200 OK!)")
    print("Expected: CPU oscillates with health - HIGH/MEDIUM/LOW cycles")
    print("=" * 60)
    sys.stdout.flush()

    app_state = ApplicationState()

    threading.Thread(target=health_server, daemon=True).start()
    print(f"[HTTP] Health server on :{HEALTH_CHECK_PORT}")
    sys.stdout.flush()

    threading.Thread(target=monitor, args=(app_state,), daemon=True).start()

    work_loop(app_state)


if __name__ == "__main__":
    main()
