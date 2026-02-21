# Agent Health Lab Tools

This directory contains tools and utilities for testing and developing agent health functionality.

These tools use the official HealthReport protobuf schema from [agent-payload](https://github.com/DataDog/agent-payload/blob/master/proto/healthplatform/healthplatform.proto).

## DDA Command: send-agenthealth

The `dda lab send-agenthealth` command allows you to send test HealthReport payloads to the Datadog event platform intake endpoint.

### Prerequisites

- A valid Datadog API key
- Access to the event platform intake endpoint

### Usage Examples

#### 1. Send to staging (default)

```bash
# Using command line option
dda lab send-agenthealth --api-key <your-api-key>

# Using environment variable
export DD_API_KEY=<your-api-key>
dda lab send-agenthealth
```

By default, the command sends to **staging** (datad0g.com).

#### 2. Send to production

```bash
dda lab send-agenthealth -k <key> --to-prod
```

#### 3. Send custom docker permission issue

```bash
dda lab send-agenthealth -k <key> \
  --issue-id docker-permission-issue \
  --issue-title "Docker Permission Error" \
  --issue-description "Cannot access Docker socket" \
  --issue-category permissions \
  --issue-severity error
```

#### 4. Send from JSON file

```bash
dda lab send-agenthealth -k <key> -f example-payload.json
```

#### 5. Send multiple payloads (e.g., for load testing)

```bash
# Send the same payload 10 times
dda lab send-agenthealth -k <key> --count 10

# Send multiple payloads from a file
dda lab send-agenthealth -k <key> -f healthreport.json -n 100
```

Each payload will have unique timestamps (emitted_at, detected_at) to ensure they are distinct.

#### 6. Send payloads continuously at intervals

```bash
# Send payloads every 5 seconds (runs until Ctrl+C)
dda lab send-agenthealth -k <key> --interval 5

# Send 100 payloads with 1 second between each
dda lab send-agenthealth -k <key> --count 100 --interval 1

# Send payloads every 0.5 seconds indefinitely
dda lab send-agenthealth -k <key> -i 0.5
```

This is useful for continuous load testing or monitoring. Press Ctrl+C to stop.

#### 7. Verbose mode for debugging

```bash
dda lab send-agenthealth -k <key> -v
```

This will show:
- Full request details (endpoint, headers, payload)
- Full response details (status, headers, body)

### Command Options

| Option | Short | Description | Default |
|--------|-------|-------------|---------|
| `--api-key` | `-k` | Datadog API key (can use DD_API_KEY env var) | Required |
| `--to-prod` | | Send to production (datadog.com) instead of staging (datad0g.com) | False |
| `--payload-file` | `-f` | Path to JSON file with full HealthReport | None |
| `--hostname` | | Hostname (defaults to system hostname) | System hostname |
| `--agent-version` | | Agent version | `test-0.0.0` |
| `--issue-id` | | Issue unique identifier | `test-issue` |
| `--issue-title` | `-t` | Issue title | `Test Issue` |
| `--issue-description` | `-d` | Issue description | `Test issue description from dda CLI` |
| `--issue-category` | | Issue category (permissions, connectivity, etc.) | `test` |
| `--issue-severity` | | Issue severity (info, warning, error, critical) | `warning` |
| `--issue-source` | | Issue source identifier | `dda-cli` |
| `--count` | `-n` | Number of payloads to send. Use 0 with --interval for infinite sends | 1 |
| `--interval` | `-i` | Interval in seconds between sends. Enables continuous sending | None |
| `--verbose` | `-v` | Show detailed request/response | False |

### Payload Format

The JSON payload follows the HealthReport protobuf schema:

```json
{
  "schema_version": "1.0.0",
  "event_type": "agent-health",
  "emitted_at": "2025-02-02T12:00:00Z",
  "host": {
    "hostname": "test-host",
    "agent_version": "7.60.0",
    "par_ids": []
  },
  "issues": {
    "issue-id": {
      "id": "issue-id",
      "issue_name": "issue-id",
      "title": "Issue Title",
      "description": "Issue description",
      "category": "permissions",
      "location": "/path/to/resource",
      "severity": "error",
      "detected_at": "2025-02-02T12:00:00Z",
      "source": "integration-name",
      "tags": ["tag1", "tag2"]
    }
  }
}
```

See [example-payload.json](./example-payload.json) for a complete example.

**Schema Reference**: [agent-payload/healthplatform.proto](https://github.com/DataDog/agent-payload/blob/master/proto/healthplatform/healthplatform.proto)

### Troubleshooting

#### API Key Issues

If you get authentication errors:
- Verify your API key is correct
- Check that you have the right permissions
- Make sure there are no extra spaces in the key

#### Connection Issues

If you get connection errors:
- Check your network connectivity
- Verify the endpoint URL is correct
- Try with verbose mode (`-v`) to see detailed error messages

#### Payload Validation

If your payload is rejected:
- Verify the JSON structure matches the expected format
- Use verbose mode to see the exact error response
- Check that all required fields are present
