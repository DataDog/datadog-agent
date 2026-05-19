# configsender (experimental, DSCVR-438 PoC)

Standalone Go tool that reads a config file from disk, redacts known
sensitive keys, builds the EvP envelope expected by the `demoalpha-worker`
([DataDog/experimental#9989](https://github.com/DataDog/experimental/pull/9989)),
and POSTs it to a configurable intake URL.

**This is not part of the agent binary.** It is a thin Go port of the bash
sender in `DataDog/experimental`, used to validate the agent-side leg of
the config-file ingestion pipeline before any Phase D architecture
decision (see the [DSCVR Roadmap](https://datadoghq.atlassian.net/wiki/spaces/DSCVR/pages/6733890585/Roadmap)).

## Build

```bash
go build -o /tmp/configsender ./pkg/discovery/tools/configsender
```

## Use

```bash
DD_API_KEY=<staging-key> /tmp/configsender \
  --intake-url=https://all-internal-intake-logs.staging.dog/v2/track/demoalpha/org/47653 \
  --host-id=$(hostname) \
  --integration=redis \
  --source=app_native \
  /etc/redis/redis.conf
```

Dry-run first to inspect the envelope without sending:

```bash
/tmp/configsender --dry-run --host-id=test --integration=redis --source=app_native /etc/redis/redis.conf
```

## What it does

1. Detects `content_type` from the file extension + integration name
   (`yaml`, `json`, `redis_conf`). Restrictive: `.conf` is only accepted
   for `integration=redis`.
2. Reads up to 256 KiB from the file.
3. Redacts values for sensitive keys (`password`, `requirepass`, `token`,
   `api_key`, etc.) — replaced with `[REDACTED]` line by line.
4. Builds the envelope: `{service, project, ddtags, message, data:{
   host_id, integration, config_source, filename, content_type, raw}}`.
5. POSTs to the configured intake URL with `DD-API-KEY` header.

## Out of scope (intentionally)

- Service discovery (use [#50811](https://github.com/DataDog/datadog-agent/pull/50811) to find paths).
- Polling / scheduling.
- Persistence / dedup across runs.
- Production transport (no forwarder integration).
- Per-file or per-integration parsers — the worker parses, not the agent.

## Roadmap context

This tool helps complete Phase A of the [DSCVR Roadmap](https://datadoghq.atlassian.net/wiki/spaces/DSCVR/pages/6733890585/Roadmap)
(end-to-end ingestion on the `demoalpha` track) without committing to a
Phase D architecture choice (in-agent collector vs. config-loader hook vs.
defer). Once the `agentdiscovery` track lands
([logs-backend#136593](https://github.com/DataDog/logs-backend/pull/136593))
the `--intake-url` flag is the only thing that changes.
