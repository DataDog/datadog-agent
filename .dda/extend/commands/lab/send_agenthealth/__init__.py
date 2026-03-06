from __future__ import annotations

import json
from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


def _build_payload(
    payload_file: str | None,
    hostname: str,
    agent_version: str,
    issue_id: str,
    issue_title: str,
    issue_description: str,
    issue_category: str,
    issue_severity: str,
    issue_source: str,
) -> dict:
    """Build or load payload for sending."""
    from datetime import datetime, timezone

    if payload_file:
        with open(payload_file) as f:
            payload = json.load(f)
        # Update timestamps for each iteration to make payloads unique
        payload["emitted_at"] = datetime.now(timezone.utc).isoformat()
        if "issues" in payload:
            for issue in payload["issues"].values():
                issue["detected_at"] = datetime.now(timezone.utc).isoformat()
    else:
        # Build HealthReport structure
        payload = {
            "schema_version": "1.0.0",
            "event_type": "agent-health",
            "emitted_at": datetime.now(timezone.utc).isoformat(),
            "host": {"hostname": hostname, "agent_version": agent_version, "par_ids": []},
            "issues": {
                issue_id: {
                    "id": issue_id,
                    "issue_name": issue_id,
                    "title": issue_title,
                    "description": issue_description,
                    "category": issue_category,
                    "location": "",
                    "severity": issue_severity,
                    "detected_at": datetime.now(timezone.utc).isoformat(),
                    "source": issue_source,
                    "tags": [],
                }
            },
        }
    return payload


def _send_single_payload(
    endpoint: str,
    api_key: str,
    payload_json: str,
) -> tuple[int, str]:
    """Send a single payload and return status code and response data."""
    import http.client
    from urllib.parse import urlparse

    parsed_url = urlparse(endpoint)
    host = parsed_url.netloc
    path = parsed_url.path

    if parsed_url.scheme == "https":
        conn = http.client.HTTPSConnection(host)
    else:
        conn = http.client.HTTPConnection(host)

    headers = {
        "Content-Type": "application/json",
        "DD-API-KEY": api_key,
    }

    conn.request("POST", path, payload_json, headers)
    response = conn.getresponse()
    response_data = response.read().decode()
    status = response.status

    conn.close()

    return status, response_data


@dynamic_command(short_help="Send agent health payload to Datadog backend")
@click.option(
    "--api-key",
    "-k",
    envvar="DD_API_KEY",
    required=True,
    help="Datadog API key (can also use DD_API_KEY env var)",
)
@click.option(
    "--to-prod",
    is_flag=True,
    default=False,
    help="Send to production (datadog.com) instead of staging (datad0g.com)",
)
@click.option(
    "--payload-file",
    "-f",
    type=click.Path(exists=True),
    help="Path to JSON file containing the payload",
)
@click.option(
    "--hostname",
    default=None,
    help="Hostname (defaults to system hostname)",
)
@click.option(
    "--agent-version",
    default="test-0.0.0",
    help="Agent version",
)
@click.option(
    "--issue-id",
    default="test-issue",
    help="Issue ID",
)
@click.option(
    "--issue-title",
    "-t",
    default="Test Issue",
    help="Issue title",
)
@click.option(
    "--issue-description",
    "-d",
    default="Test issue description from dda CLI",
    help="Issue description",
)
@click.option(
    "--issue-category",
    default="test",
    help="Issue category (e.g., permissions, connectivity)",
)
@click.option(
    "--issue-severity",
    default="warning",
    type=click.Choice(["info", "warning", "error", "critical"]),
    help="Issue severity",
)
@click.option(
    "--issue-source",
    default="dda-cli",
    help="Issue source identifier",
)
@click.option(
    "--verbose",
    "-v",
    is_flag=True,
    help="Show verbose output including request/response details",
)
@click.option(
    "--count",
    "-n",
    default=1,
    type=int,
    help="Number of payloads to send (default: 1). Use 0 with --interval for infinite sends.",
)
@click.option(
    "--interval",
    "-i",
    type=float,
    help="Interval in seconds between sends. If count=1 (default), sends indefinitely until Ctrl+C.",
)
@pass_app
def cmd(
    app: Application,
    *,
    api_key: str,
    to_prod: bool,
    payload_file: str | None,
    hostname: str | None,
    agent_version: str,
    issue_id: str,
    issue_title: str,
    issue_description: str,
    issue_category: str,
    issue_severity: str,
    issue_source: str,
    verbose: bool,
    count: int,
    interval: float | None,
) -> None:
    """
    Send agent health payload to Datadog backend.

    This command sends HealthReport payloads following the agent-payload protobuf schema
    to the Datadog event platform intake endpoint for testing and validation.

    By default, sends to staging (datad0g.com). Use --to-prod to send to production.

    Examples:

        # Send default test issue to staging
        dda lab send-agenthealth --api-key <your-key>

        # Send to production
        dda lab send-agenthealth -k <key> --to-prod

        # Send custom docker permission issue
        dda lab send-agenthealth -k <key> \\
            --issue-id docker-permission-issue \\
            --issue-title "Docker Permission Error" \\
            --issue-category permissions \\
            --issue-severity error

        # Send from JSON file (full HealthReport structure)
        dda lab send-agenthealth -k <key> -f healthreport.json

        # Send multiple payloads (e.g., for load testing)
        dda lab send-agenthealth -k <key> --count 10

        # Send payloads continuously every 5 seconds (until Ctrl+C)
        dda lab send-agenthealth -k <key> --interval 5

        # Send 100 payloads with 1 second between each
        dda lab send-agenthealth -k <key> --count 100 --interval 1

        # Verbose mode to see request/response
        dda lab send-agenthealth -k <key> -v
    """
    import socket
    import time

    # Validate parameters
    if count < 0:
        app.display_error("❌ Count must be 0 or greater")
        raise SystemExit(1)

    if interval is not None and interval <= 0:
        app.display_error("❌ Interval must be positive")
        raise SystemExit(1)

    # Determine if running indefinitely
    run_indefinitely = interval is not None and count <= 1

    if run_indefinitely:
        actual_count = float("inf")  # Run until interrupted
    elif count == 0:
        app.display_error("❌ Count of 0 is only valid with --interval")
        raise SystemExit(1)
    else:
        actual_count = count

    # Build endpoint URL based to_prod flag
    domain = "datadog.com" if to_prod else "datad0g.com"
    endpoint = f"https://event-platform-intake.{domain}/api/v2/agenthealth"

    # Get hostname if not provided
    if hostname is None:
        hostname = socket.gethostname()

    # Display summary
    if run_indefinitely:
        app.display_info(f"Sending payloads every {interval}s to {endpoint} (Ctrl+C to stop)")
    elif count > 1:
        if interval:
            app.display_info(f"Sending {count} payloads every {interval}s to {endpoint}")
        else:
            app.display_info(f"Sending {count} payloads to {endpoint}")

    success_count = 0
    failed_count = 0
    i = 0

    try:
        # Send payloads
        while i < actual_count:
            # Build payload for this iteration
            if payload_file and i == 0:
                app.display_info(f"Using payload from file: {payload_file}")

            payload = _build_payload(
                payload_file,
                hostname,
                agent_version,
                issue_id,
                issue_title,
                issue_description,
                issue_category,
                issue_severity,
                issue_source,
            )
            payload_json = json.dumps(payload, indent=2)

            # Show verbose output for first payload only (or all if count=1)
            if verbose and (i == 0 or count == 1):
                app.display_info("Request Details:")
                app.display_info(f"  Endpoint: {endpoint}")
                masked_key = f"{'*' * (len(api_key) - 4)}{api_key[-4:]}" if len(api_key) > 4 else "****"
                app.display_info(f"  API key: {masked_key}")
                app.display_info(f"  Payload:\n{payload_json}")

            # Make request
            try:
                status, response_data = _send_single_payload(endpoint, api_key, payload_json)

                # Show verbose output for first payload only (or all if count=1)
                if verbose and (i == 0 or count == 1):
                    app.display_info("\nResponse Details:")
                    app.display_info(f"  Status: {status}")
                    app.display_info(f"  Body: {response_data}")

                if 200 <= status < 300:
                    success_count += 1
                    if count == 1 and not interval:
                        app.display_success(f"✅ Successfully sent agent health payload (status: {status})")
                        if not verbose and response_data:
                            app.display_info(f"Response: {response_data}")
                    elif verbose or (not run_indefinitely and count <= 10):
                        if run_indefinitely:
                            app.display_success(f"✅ Payload #{i + 1} sent successfully")
                        else:
                            app.display_success(f"✅ Payload {i + 1}/{count} sent successfully")
                else:
                    failed_count += 1
                    if count == 1 and not interval:
                        app.display_error(f"❌ Failed to send payload (status: {status})")
                        app.display_error(f"Response: {response_data}")
                        raise SystemExit(1) from None
                    else:
                        if run_indefinitely:
                            app.display_error(f"❌ Payload #{i + 1} failed (status: {status})")
                        else:
                            app.display_error(f"❌ Payload {i + 1}/{count} failed (status: {status})")

            except Exception as e:
                failed_count += 1
                if count == 1 and not interval:
                    app.display_error(f"❌ Error sending payload: {e}")
                    raise SystemExit(1) from None
                else:
                    if run_indefinitely:
                        app.display_error(f"❌ Payload #{i + 1} error: {e}")
                    else:
                        app.display_error(f"❌ Payload {i + 1}/{count} error: {e}")

            # Increment counter
            i += 1

            # Sleep between payloads if interval is set and not the last iteration
            if interval and i < actual_count:
                time.sleep(interval)

    except KeyboardInterrupt:
        app.display_info("\n\n⚠️  Interrupted by user (Ctrl+C)")

    # Display final summary
    if count > 1 or run_indefinitely:
        app.display_info(f"\nSummary: {success_count} succeeded, {failed_count} failed out of {i} total")
        if failed_count > 0:
            raise SystemExit(1) from None
