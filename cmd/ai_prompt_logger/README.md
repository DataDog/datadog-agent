# AI usage native messaging host (Rust)

Chrome **Native Messaging** host that forwards **AI usage** events to the **local Datadog Agent** via the trace receiver **EVP proxy** (Logs v2), not directly to Datadog with a customer API key.

Standalone **`install_mac.sh` / `install.ps1`** helpers are **not** part of this tree; Chrome registration for macOS is done by the **Datadog Agent** package (`postinst` + per-user manifests). Windows packaging will follow Agent conventions when added.

## Build

From the **repository root** (workspace member):

```bash
cargo build -p ai-prompt-logger-native-host --release
```

Binary: `target/release/ai-prompt-logger-native-host` (`.exe` on Windows).

With Bazel (same as CI / omnibus for macOS):

```bash
bazelisk build --config=release //cmd/ai_prompt_logger:ai-prompt-logger-native-host
```

## Configuration

Settings live in **`ai_usage_native_host.yaml`** (see **`ai_usage_native_host.yaml.example`**).

- **Explicit file** (same idea as `agent run -c` / `system-probe --config`):

  ```bash
  ai-prompt-logger-native-host --config=/opt/datadog-agent/etc/ai_usage_native_host.yaml
  ai-prompt-logger-native-host -c ./ai_usage_native_host.yaml
  ```

- **No flags**: the binary looks for `ai_usage_native_host.yaml` under `{install_root}/etc/…` inferred from `…/embedded/bin/<executable>` (packaged layout).

On a **packaged macOS** agent, Chrome is pointed at **`embedded/bin/run_ai_usage_native_host.sh`**, which **`exec`s** the binary with **`--config=$install_root/etc/ai_usage_native_host.yaml`**.

EVP / Agent URL behaviour (defaults):

- URL: `{trace_agent_url}/evp_proxy/v{evp_proxy_api_version}/api/v2/logs` (defaults match trace receiver / EVP v2).
- Header: `X-Datadog-EVP-Subdomain: {logs_evp_subdomain}` (default `http-intake.logs`).
- The Agent injects `DD-API-KEY` and forwards to the correct Logs intake for your site.

Ensure the Agent is listening on the trace port (default `localhost:8126`) with EVP proxy enabled.

## Protocol

Chrome uses a 4-byte little-endian length prefix + UTF-8 JSON for each message.

### `HEALTH_CHECK`

Request:

```json
{ "type": "HEALTH_CHECK" }
```

Response:

```json
{ "type": "HEALTH_RESULT", "status": "ok" }
```

### `SEND_USAGE_EVENT`

Request (shape used by the AI usage extension):

```json
{
  "type": "SEND_USAGE_EVENT",
  "tool": "example",
  "user_id": "user-1",
  "approved": true
}
```

Optional fields: `provider`, `hostname`.

Response:

```json
{ "type": "SEND_USAGE_EVENT_RESULT", "success": true }
```

## Local smoke test

From the repo root, after a release build (script defaults to `target/release/…` under the repo):

```bash
python3 cmd/ai_prompt_logger/scripts/test_host.py
# or pass the binary explicitly:
python3 cmd/ai_prompt_logger/scripts/test_host.py target/release/ai-prompt-logger-native-host
```

With an explicit config file (any path the host can read):

```bash
python3 cmd/ai_prompt_logger/scripts/test_host.py target/release/ai-prompt-logger-native-host --config=/path/to/ai_usage_native_host.yaml
```

`SEND_USAGE_EVENT` reports `success: true` only if the local Agent accepts the EVP/logs request.
