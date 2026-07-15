# AI usage native messaging host (Rust)

Chrome **Native Messaging** host that forwards **AI usage** events to the **local Datadog Agent** via the trace receiver **EVP proxy** and the dedicated AI usage track, not directly to Datadog with a customer API key.

Chrome registration for **macOS** is done by the **Datadog Agent** package (`postinst` + per-user manifests). On **Windows**, the MSI installs a machine-wide Chrome Native Messaging Host registration under HKLM — see the **Windows install** section below.

The same binary can also run as a standalone desktop monitor with `--desktop-monitor`. In that mode it is a separate long-lived user-session process and does not use Chrome's native messaging protocol.

## Build

From the **repository root** (workspace member):

```bash
cargo build -p ai-usage-agent-native-host --release
```

Binary: `target/release/ai-usage-agent-native-host` (`.exe` on Windows).

With Bazel (same as CI / omnibus for macOS):

```bash
bazelisk build --config=release //cmd/ai_prompt_logger:ai-usage-agent-native-host
```

## Configuration

Settings live in **`ai_usage_native_host.yaml`** (see **`ai_usage_native_host.yaml.example`**).

- **Explicit file** (same idea as `agent run -c` / `system-probe --config`):

  ```bash
  ai-usage-agent-native-host --config=/opt/datadog-agent/etc/ai_usage_native_host.yaml
  ai-usage-agent-native-host -c ./ai_usage_native_host.yaml
  ai-usage-agent-native-host --desktop-monitor -c ./ai_usage_native_host.yaml
  ```

- **No flags**: the binary looks for `ai_usage_native_host.yaml` under the packaged config location. On Windows, it first uses the Agent MSI `ConfigRoot` registry value, then falls back to `%ProgramData%\Datadog\ai_usage_native_host.yaml`; otherwise it searches under `{install_root}/etc/…` inferred from the executable path.

On a **packaged macOS** agent, Chrome is pointed at **`embedded/bin/run_ai_usage_native_host.sh`**, which **`exec`s** the binary with **`--config=$install_root/etc/ai_usage_native_host.yaml`**. On first config creation, the installer generates `trace_agent_url` from the Agent's `apm_config.receiver_port` in `datadog.yaml`.

On a **packaged Windows** agent (MSI), Chrome is pointed directly at **`bin\agent\ai-usage-agent-native-host.exe`** through a machine-wide HKLM Native Messaging Host registration; no shell wrapper is used.

EVP / Agent URL behaviour (defaults):

- URL: `{trace_agent_url}/evp_proxy/v{evp_proxy_api_version}/api/v2/aiusage` (defaults match trace receiver / EVP v2).
- Header: `X-Datadog-EVP-Subdomain: {ai_usage_evp_subdomain}` (default `softinv-intake`).
- The Agent injects `DD-API-KEY` and forwards to the dedicated AI usage intake for your site.

Ensure the Agent is listening on the trace port (default `127.0.0.1:8126`) with EVP proxy enabled.

## Desktop monitor mode

`--desktop-monitor` runs the host as a standalone desktop application. It is intended to be launched in the interactive user session, not as the Datadog Agent or system-probe service in Session 0.

On Windows and macOS, each poll:

1. Reads the foreground window.
2. Resolves the foreground process image name.
3. Directly matches the foreground process against the configured AI process lookup table.
4. If the foreground process is a configured host process such as a terminal or IDE, scans the process tree for AI CLI descendants. Hosted candidates emit only after their process-level read/write counters advance since the previous poll; the first poll initializes a baseline.
5. Sends at most one observed AI usage event for the poll through the same Agent EVP path used by Chrome mode.

Relevant config keys under `desktop_monitoring`:

- `enabled`: disables standalone monitoring when set to `false`.
- `poll_interval_seconds`: poll interval, default `60`.
- `ai_process_names`: custom AI app/CLI lookup entries. Built-in defaults are compiled into the monitor and include Cursor, Claude/Claude Code, Claude Cowork service, Codex, OpenClaw, Hermes Agent, and additional Agent Skills client candidates. YAML entries are unioned with built-ins; a YAML entry with the same `tool` value replaces the built-in record.
- `ai_process_names[].match_scope`: controls whether an entry applies to direct foreground processes (`direct`), hosted child processes (`hosted_child`), or both (`both`). Matching is case-insensitive, so duplicate process names that only differ by case are unnecessary.
- `process_activity_window_seconds`: read/write activity observation window, default `600`.
- `host_process_names`: custom foreground host lookup entries for terminals and IDEs that may contain AI CLI children. YAML entries are merged with the built-in host list.

Broad runtime names such as `node.exe`/`node`, `python.exe`/`python`, and `conhost.exe` are not direct AI-tool matches by default because they need command-line or path inspection to avoid false positives.

Desktop events use the extension-compatible field semantics: tool display name, provider, user ID, and approved flag. The event source is `desktop_app`.

On Windows, `--desktop-monitor` detaches from the scheduler-created console after startup. When file logging is enabled for diagnostics or startup config errors need to be reported, logs are written to `C:\ProgramData\Datadog\logs\ai-usage-desktop-monitor.log`, falling back to `%LOCALAPPDATA%\Datadog\logs\ai-usage-desktop-monitor.log` if the ProgramData log path is unavailable. Each record includes the process ID and user. The log rotates at 10 MB with one `.1` backup.

On macOS, foreground detection uses the frontmost visible CoreGraphics window owner. Terminal metadata remains available in diagnostics, but hosted CLI detection is driven by process read/write counter deltas. When file logging is enabled for diagnostics or startup config errors need to be reported, logs are written to `$DD_LOG_DIR` when set, then `/opt/datadog-agent/logs/ai-usage-desktop-monitor.log`, then `~/Library/Logs/Datadog/ai-usage-desktop-monitor.log`.

## Protocol

Chrome uses a 4-byte little-endian length prefix + UTF-8 JSON for each message.
The native host keeps stdout reserved for protocol frames and reads Chrome messages from stdin. Runtime diagnostics do not write to stdout or stderr.

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

Optional field: `provider`.

Response:

```json
{ "type": "SEND_USAGE_EVENT_RESULT", "success": true }
```

## Windows install (MSI)

The Datadog Agent Windows MSI ships the native host under the Agent install directory and registers it with Chrome machine-wide:

| Path (default install) | Contents |
|---|---|
| `C:\Program Files\Datadog\Datadog Agent\bin\agent\ai-usage-agent-native-host.exe` | The host binary (signed). |
| `C:\Program Files\Datadog\Datadog Agent\bin\agent\dist\com.datadoghq.ai_usage_agent.native_host.json` | Chrome Native Messaging Host manifest generated by the MSI. |
| `C:\ProgramData\Datadog\ai_usage_native_host.yaml.example` | Config example. |
| `C:\ProgramData\Datadog\ai_usage_native_host.yaml` | Active config generated by the MSI if missing. |

The MSI writes machine-wide registry keys so every Chrome user on the machine can discover the same native host:

```
HKLM\SOFTWARE\Google\Chrome\NativeMessagingHosts\com.datadoghq.ai_usage_agent.native_host
HKLM\SOFTWARE\WOW6432Node\Google\Chrome\NativeMessagingHosts\com.datadoghq.ai_usage_agent.native_host
```

The default value for both keys points to the manifest under `bin\agent\dist`. The manifest's `path` field points to the bundled host executable under `bin\agent`, and `allowed_origins` uses the installer default Chrome extension ID (`gkmbhgbippkmmmidcikijiblbagbjgjj`). The active config's `trace_agent_url` is generated from the Agent's `apm_config.receiver_port` in `datadog.yaml`.

The MSI registers a Task Scheduler logon task named `Datadog AI Usage Agent`. The task launches the same bundled host executable with `--desktop-monitor --config "C:\ProgramData\Datadog\ai_usage_native_host.yaml"` in the interactive user session. Chrome native messaging registration remains separate and continues to launch the host without `--desktop-monitor`. When the `DatadogAgent` service is stopped, the desktop monitor remains resident but skips desktop scans until SCM reports the service running again.

## macOS install (DMG)

The Datadog Agent macOS package ships the native host under `/opt/datadog-agent/embedded/bin` and installs the Chrome native messaging wrapper `run_ai_usage_native_host.sh`.

For desktop monitoring, the package also ships `com.datadoghq.ai-usage-agent.desktop-monitor.plist.example` and installs it as a LaunchAgent with label `com.datadoghq.ai-usage-agent.desktop-monitor`. The LaunchAgent runs:

```bash
/opt/datadog-agent/embedded/bin/ai-usage-agent-native-host --desktop-monitor --config /opt/datadog-agent/etc/ai_usage_native_host.yaml
```

The LaunchAgent uses `RunAtLoad` and `KeepAlive` for non-successful exits. Package scripts bootstrap it for the active console user on install, boot it out during upgrade/uninstall, and remove it on uninstall. The monitor checks `system/com.datadoghq.agent` through `launchctl`; while the Agent service is stopped, it remains resident but skips desktop scans until the service is running again.

The package grants narrow read access for logged-in users to `ai_usage_native_host.yaml` without broadening access to unrelated Agent config files.

## Local smoke test

From the repo root, after a release build (script defaults to `target/release/…` under the repo):

```bash
python3 cmd/ai_prompt_logger/scripts/test_host.py
# or pass the binary explicitly:
python3 cmd/ai_prompt_logger/scripts/test_host.py target/release/ai-usage-agent-native-host
```

With an explicit config file (any path the host can read):

```bash
python3 cmd/ai_prompt_logger/scripts/test_host.py target/release/ai-usage-agent-native-host --config=/path/to/ai_usage_native_host.yaml
```

`SEND_USAGE_EVENT` reports `success: true` only if the local Agent accepts the EVP AI usage request.

For local desktop-monitoring validation, prefer Rust unit tests plus manual Windows or macOS smoke tests. Bazel/package verification is useful in CI or a provisioned packaging environment, but Bazel-related tools may not be available on every local workstation.
