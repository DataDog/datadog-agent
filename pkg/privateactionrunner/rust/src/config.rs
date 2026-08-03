// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Configuration for par-control: the OPMS endpoint, concurrency, socket paths,
//! and timing. Loaded from the agent `datadog.yaml` (only the fields the control
//! plane needs are deserialized) plus the persisted runner identity.

use crate::identity::Identity;
use anyhow::{Context, Result, bail};
use std::path::{Path, PathBuf};
use std::time::Duration;

/// Default local sockets. The executor socket must match the Go executor's
/// `private_action_runner.executor.socket_path` platform default; the procmgr
/// socket must match dd-procmgrd's own default (`pkg/procmgr/rust/src/transport/`).
/// Both are per-platform: a Unix socket path on Unix, a named pipe on Windows.
#[cfg(not(windows))]
pub const DEFAULT_EXECUTOR_SOCKET: &str = "/opt/datadog-agent/run/par-executor.sock";
#[cfg(windows)]
pub const DEFAULT_EXECUTOR_SOCKET: &str = r"\\.\pipe\dd-par-executor";
#[cfg(not(windows))]
pub const DEFAULT_PROCMGR_SOCKET: &str = "/var/run/datadog-procmgrd/dd-procmgrd.sock";
#[cfg(windows)]
pub const DEFAULT_PROCMGR_SOCKET: &str = r"\\.\pipe\datadog-procmgrd";
/// Process-definition name the executor is registered under with the process
/// manager. MUST match the `name` of the procmgr process definition installed by
/// `pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-action-executor.yaml.tmpl`
/// (see the generated files under `.../tmpl/gen/`), otherwise every
/// Start/Describe/Stop RPC fails with an unknown-process error.
pub const DEFAULT_EXECUTOR_PROCESS_NAME: &str = "datadog-agent-action-executor";

/// PAR flavor string sent to OPMS (matches `flavor.PrivateActionRunner` on the Go side).
pub const FLAVOR: &str = "private_action_runner";

/// Runner version reported to OPMS in the `X-Datadog-OnPrem-Version` header.
///
/// Must be the *agent* version (Go reports `version.AgentVersion`), not the
/// crate version — OPMS uses this to reason about runner capabilities. Bazel
/// injects `DD_AGENT_VERSION` via `rustc_env` in BUILD.bazel, computed by the
/// crate-local `version.bzl`. Under a plain `cargo` build (no Bazel) the var is
/// absent and we fall back to the crate version.
pub const RUNNER_VERSION: &str = match option_env!("DD_AGENT_VERSION") {
    Some(v) => v,
    None => env!("CARGO_PKG_VERSION"),
};

#[derive(Clone)]
pub struct Config {
    pub opms_base_url: String,
    /// Action worker-pool size, from `private_action_runner.task_concurrency`.
    pub task_concurrency: usize,
    pub executor_socket: PathBuf,
    pub procmgr_socket: PathBuf,
    pub executor_process_name: String,
    pub loop_interval: Duration,
    /// Only while an action's result stream is open.
    pub heartbeat_interval: Duration,
    pub idle_timeout: Duration,
    pub ready_timeout: Duration,
    /// Bounds the initial cold-executor wait for verified RC keys.
    pub key_sync_timeout: Duration,
    pub opms_request_timeout: Duration,
    /// Mirrors the Go circuit breaker.
    pub min_backoff: Duration,
    pub max_backoff: Duration,
    pub wait_before_retry: Duration,
    pub max_attempts: u32,
    pub runner_version: String,
    /// Comma-joined in the OPMS header.
    pub modes: Vec<String>,
    /// Resolved path of the agent IPC cert used as the control<->executor mTLS
    /// identity. Always set: the executor unconditionally requires a client cert,
    /// so there is no valid "no mTLS" mode to represent with an `Option`. The file
    /// itself may not exist yet when par-control starts (see
    /// [`crate::transport::connect_lazy_tls`]).
    pub ipc_cert_file: PathBuf,
    pub identity: Identity,
}

/// Minimal view of `datadog.yaml` — only the fields the control plane reads.
/// Unknown keys are ignored so the full agent config deserializes cleanly.
#[derive(serde::Deserialize, Default, Clone)]
struct RawConfig {
    site: Option<String>,
    /// Agent-wide `log_level`. Reused verbatim instead of adding a par-control
    /// specific key, so one operator-facing switch controls both halves.
    log_level: Option<String>,
    /// Full DD URL override, used by e2e against a plaintext fake OPMS.
    dd_url: Option<String>,
    ipc_cert_file_path: Option<String>,
    /// Only used to locate the IPC cert when `ipc_cert_file_path` is unset,
    /// mirroring the Go resolution order.
    auth_token_file_path: Option<String>,
    private_action_runner: Option<RawPar>,
}

#[derive(serde::Deserialize, Default, Clone)]
struct RawPar {
    /// `private_action_runner.enabled`.
    enabled: Option<bool>,
    /// `private_action_runner.split_enabled`: run the split control plane +
    /// on-demand executor instead of the monolithic Go runner.
    split_enabled: Option<bool>,
    /// `private_action_runner.self_enroll` (default true on the Go side).
    self_enroll: Option<bool>,
    urn: Option<String>,
    private_key: Option<String>,
    /// Size of the action worker pool. Same key and default (5) as the Go
    /// runner's `private_action_runner.task_concurrency`, so both deployment
    /// modes honour one operator-facing setting.
    task_concurrency: Option<usize>,
    /// Nested `private_action_runner.executor.*` block, matching the Go config
    /// key `private_action_runner.executor.socket_path`.
    executor: Option<RawParExecutor>,
    procmgr_socket_path: Option<String>,
    executor_process_name: Option<String>,
    identity_file_path: Option<String>,
    idle_timeout_seconds: Option<u64>,
    heartbeat_interval_seconds: Option<u64>,
    #[serde(default)]
    modes: Vec<String>,
}

/// The `private_action_runner.executor.*` sub-section.
#[derive(serde::Deserialize, Default, Clone)]
struct RawParExecutor {
    socket_path: Option<String>,
}

impl Config {
    /// Load configuration from the given `datadog.yaml` path, erroring if the
    /// runner has no identity yet (use [`Config::try_from_yaml_file`] to detect
    /// the not-yet-enrolled case for bootstrap).
    pub fn from_yaml_file(path: &std::path::Path) -> Result<Self> {
        Self::try_from_yaml_file(path)?
            .context("runner is not enrolled: no inline identity and no identity file found")
    }

    /// Like [`Config::from_yaml_file`] but returns `Ok(None)` when the config is
    /// otherwise valid yet no runner identity is present, so the caller can run
    /// the Go one-shot enroll and retry.
    pub fn try_from_yaml_file(path: &std::path::Path) -> Result<Option<Self>> {
        let contents = std::fs::read_to_string(path)
            .with_context(|| format!("failed to read config file: {}", path.display()))?;
        let raw: RawConfig =
            serde_yaml::from_str(&contents).context("failed to parse datadog.yaml")?;

        // Resolve identity from inline config keys, else the persisted identity
        // file (explicit path, else next to datadog.yaml) that Go enrollment writes.
        let identity = match inline_identity(raw.private_action_runner.as_ref())? {
            Some(id) => Some(id),
            None => Identity::from_file(&identity_file_path(&raw, path))?,
        };
        match identity {
            Some(identity) => Ok(Some(Self::build(raw, identity, path)?)),
            None => Ok(None),
        }
    }

    /// Parse configuration from YAML contents with inline identity (test helper).
    /// Paths that default relative to the config file resolve against a notional
    /// `datadog.yaml` in the current directory.
    pub fn from_yaml_str(yaml: &str) -> Result<Self> {
        let raw: RawConfig = serde_yaml::from_str(yaml).context("failed to parse datadog.yaml")?;
        let identity = inline_identity(raw.private_action_runner.as_ref())?
            .context("private_action_runner identity is not set")?;
        Self::build(raw, identity, Path::new("datadog.yaml"))
    }

    fn build(raw: RawConfig, identity: Identity, config_path: &Path) -> Result<Self> {
        let par = raw.private_action_runner.clone().unwrap_or_default();
        let opms_base_url = resolve_opms_base_url(raw.site.as_deref(), raw.dd_url.as_deref())?;

        Ok(Config {
            opms_base_url,
            // Default 5 matches the Go `private_action_runner.task_concurrency`
            // default in pkg/config/setup/privateactionrunner_settings.go.
            task_concurrency: par.task_concurrency.filter(|n| *n > 0).unwrap_or(5),
            executor_socket: PathBuf::from(
                par.executor
                    .and_then(|e| e.socket_path)
                    .filter(|s| !s.is_empty())
                    .unwrap_or_else(|| DEFAULT_EXECUTOR_SOCKET.to_string()),
            ),
            procmgr_socket: PathBuf::from(
                par.procmgr_socket_path
                    .unwrap_or_else(|| DEFAULT_PROCMGR_SOCKET.to_string()),
            ),
            executor_process_name: par
                .executor_process_name
                .unwrap_or_else(|| DEFAULT_EXECUTOR_PROCESS_NAME.to_string()),
            loop_interval: Duration::from_secs(1),
            heartbeat_interval: Duration::from_secs(par.heartbeat_interval_seconds.unwrap_or(20)),
            idle_timeout: Duration::from_secs(par.idle_timeout_seconds.unwrap_or(60)),
            ready_timeout: Duration::from_secs(10),
            key_sync_timeout: Duration::from_secs(120),
            opms_request_timeout: Duration::from_secs(30),
            // Backoff defaults mirror pkg/privateactionrunner/adapters/config/constants.go.
            min_backoff: Duration::from_secs(1),
            max_backoff: Duration::from_secs(180),
            wait_before_retry: Duration::from_secs(300),
            max_attempts: 20,
            runner_version: RUNNER_VERSION.to_string(),
            modes: par.modes,
            ipc_cert_file: ipc_cert_file_path(&raw, config_path),
            identity,
        })
    }
}

/// The launch-time gate, resolved from `datadog.yaml` *before* a runner identity
/// is required.
///
/// The launch path is unconditional: the procmgr definition that starts
/// par-control is installed on every host that ships the binary, and the Go
/// monolith's own unit is always installed too. Both dequeue from OPMS, so
/// exactly one may run — they consult the same two config keys and the loser
/// exits cleanly (see `isSplitEnabled` in
/// `comp/privateactionrunner/impl/privateactionrunner.go`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LaunchGate {
    /// `private_action_runner.enabled` && `private_action_runner.split_enabled`.
    /// Both default to false, so a host that has never opted in never starts the
    /// control plane.
    pub split_mode: bool,
    /// `private_action_runner.self_enroll` (default true, matching Go): may
    /// par-control run the Go one-shot enroll when no identity is persisted yet?
    pub self_enroll: bool,
}

/// Resolve the control plane's log level from the agent's existing `log_level`
/// key, so no par-control-specific key (and no config-schema entry) is needed.
///
/// Called before anything else in `main`, and therefore *before* any logger
/// exists: an unreadable or malformed config cannot be reported, so it degrades
/// to the default level and lets the subsequent [`LaunchGate::from_yaml_file`]
/// surface the real error.
pub fn log_level_from_yaml_file(path: &Path) -> log::Level {
    std::fs::read_to_string(path)
        .ok()
        .and_then(|contents| serde_yaml::from_str::<RawConfig>(&contents).ok())
        .and_then(|raw| raw.log_level)
        .as_deref()
        .map(parse_log_level)
        .unwrap_or(log::Level::Info)
}

/// Map an agent `log_level` string onto a `log::Level`.
///
/// The agent accepts levels the `log` crate does not model: `critical` collapses
/// onto `Error` (the most severe level available) and `off` onto `Error` as well,
/// because `dd_agent_log::init` takes a `Level` rather than a `LevelFilter` and so
/// cannot silence output entirely. Unknown values fall back to `Info` rather than
/// failing startup over a cosmetic setting.
fn parse_log_level(raw: &str) -> log::Level {
    match raw.trim().to_ascii_lowercase().as_str() {
        "trace" => log::Level::Trace,
        "debug" => log::Level::Debug,
        "warn" | "warning" => log::Level::Warn,
        "error" | "critical" | "off" => log::Level::Error,
        _ => log::Level::Info,
    }
}

impl LaunchGate {
    pub fn from_yaml_file(path: &Path) -> Result<Self> {
        let contents = std::fs::read_to_string(path)
            .with_context(|| format!("failed to read config file: {}", path.display()))?;
        Self::from_yaml_str(&contents)
    }

    pub fn from_yaml_str(yaml: &str) -> Result<Self> {
        let raw: RawConfig = serde_yaml::from_str(yaml).context("failed to parse datadog.yaml")?;
        let par = raw.private_action_runner.unwrap_or_default();
        Ok(Self {
            split_mode: par.enabled.unwrap_or(false) && par.split_enabled.unwrap_or(false),
            self_enroll: par.self_enroll.unwrap_or(true),
        })
    }
}

/// Resolve the OPMS base URL. Production uses `https://api.<site>`. A `dd_url`
/// starting with `http://` is honored verbatim so e2e tests can point at a
/// plaintext fake OPMS (mirrors the Go client's endpointURL behavior).
fn resolve_opms_base_url(site: Option<&str>, dd_url: Option<&str>) -> Result<String> {
    if let Some(url) = dd_url
        && let Some(host) = url.strip_prefix("http://")
    {
        return Ok(format!("http://{}", host.trim_end_matches('/')));
    }
    let site = site.unwrap_or("datadoghq.com");
    if site.is_empty() {
        bail!("site is empty and no dd_url override provided");
    }
    Ok(format!("https://api.{site}"))
}

/// Identity from the inline `datadog.yaml` keys, if both are present.
fn inline_identity(par: Option<&RawPar>) -> Result<Option<Identity>> {
    let Some(par) = par else {
        return Ok(None);
    };
    let urn = par.urn.as_deref().filter(|s| !s.is_empty());
    let key = par.private_key.as_deref().filter(|s| !s.is_empty());
    match (urn, key) {
        (Some(urn), Some(key)) => Ok(Some(Identity::new(urn.to_string(), key.to_string())?)),
        _ => Ok(None),
    }
}

/// Default name of the agent IPC cert file, matching `defaultCertFileName` in
/// `pkg/api/security/cert/cert_getter.go`.
const DEFAULT_IPC_CERT_FILE_NAME: &str = "ipc_cert.pem";

/// Resolve the agent IPC cert path exactly as `getCertFilepath` does on the Go
/// side, because `ipc_cert_file_path` defaults to empty and is therefore unset on
/// virtually every real host:
///
/// 1. explicit `ipc_cert_file_path`,
/// 2. else next to `auth_token_file_path` (operators who move the auth token off
///    the config directory get the cert moved with it),
/// 3. else next to `datadog.yaml`.
///
/// Getting this wrong is silent: par-control would poll OPMS happily and fail
/// every dispatch on the executor's required-client-cert handshake.
fn ipc_cert_file_path(raw: &RawConfig, config_path: &Path) -> PathBuf {
    if let Some(explicit) = raw.ipc_cert_file_path.as_deref().filter(|s| !s.is_empty()) {
        return PathBuf::from(explicit);
    }
    if let Some(auth_token) = raw.auth_token_file_path.as_deref().filter(|s| !s.is_empty()) {
        return Path::new(auth_token)
            .parent()
            .unwrap_or_else(|| Path::new("."))
            .join(DEFAULT_IPC_CERT_FILE_NAME);
    }
    config_path
        .parent()
        .unwrap_or_else(|| Path::new("."))
        .join(DEFAULT_IPC_CERT_FILE_NAME)
}

/// Path to the persisted identity file: the explicit `identity_file_path` if set,
/// else `<config dir>/privateactionrunner_private_identity.json` (matching Go's
/// default next to datadog.yaml).
fn identity_file_path(raw: &RawConfig, config_path: &Path) -> PathBuf {
    if let Some(explicit) = raw
        .private_action_runner
        .as_ref()
        .and_then(|p| p.identity_file_path.as_deref())
        .filter(|s| !s.is_empty())
    {
        return PathBuf::from(explicit);
    }
    let dir = config_path.parent().unwrap_or_else(|| Path::new("."));
    dir.join(crate::identity::DEFAULT_IDENTITY_FILE_NAME)
}

#[cfg(test)]
mod tests {
    use super::*;

    const MIN_YAML: &str = r#"
site: datadoghq.com
private_action_runner:
  urn: "urn:dd:apps:on-prem-runner:us1:42:runner-1"
  private_key: "-----BEGIN EC PRIVATE KEY-----\nabc\n-----END EC PRIVATE KEY-----"
"#;

    #[test]
    fn loads_minimal_config() {
        let cfg = Config::from_yaml_str(MIN_YAML).unwrap();
        assert_eq!(cfg.opms_base_url, "https://api.datadoghq.com");
        // Same default as Go's private_action_runner.task_concurrency.
        assert_eq!(cfg.task_concurrency, 5);
        assert_eq!(cfg.identity.org_id, 42);
        assert_eq!(cfg.identity.runner_id, "runner-1");
        // Must match the procmgr process-definition name installed by the
        // installer, else Start/Describe/Stop fail against dd-procmgrd.
        assert_eq!(
            cfg.executor_process_name,
            "datadog-agent-action-executor"
        );
        assert_eq!(cfg.executor_socket, PathBuf::from(DEFAULT_EXECUTOR_SOCKET));
    }

    /// The executor socket lives under the *nested* `executor:` block, matching the
    /// Go config key `private_action_runner.executor.socket_path`. A flat
    /// `executor_socket_path` key must be ignored, so par-control and the Go
    /// executor can never disagree about which socket to use.
    #[test]
    fn reads_nested_executor_socket_path() {
        let yaml =
            format!("{MIN_YAML}  executor:\n    socket_path: /tmp/custom-executor.sock\n");
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(
            cfg.executor_socket,
            PathBuf::from("/tmp/custom-executor.sock")
        );
    }

    /// OPMS keys runner capabilities off `X-Datadog-OnPrem-Version`, so par-control
    /// must report the agent version that Bazel injects, never the crate version.
    /// Under Bazel `DD_AGENT_VERSION` is always set (see BUILD.bazel).
    #[test]
    #[cfg(bazel)]
    fn runner_version_is_the_injected_agent_version() {
        let injected = option_env!("DD_AGENT_VERSION")
            .expect("Bazel must inject DD_AGENT_VERSION via rustc_env");
        assert_eq!(RUNNER_VERSION, injected);
        assert_ne!(
            RUNNER_VERSION,
            env!("CARGO_PKG_VERSION"),
            "reported version must be the agent version, not the crate version"
        );

        // Guard against version.bzl silently yielding an empty or malformed string:
        // `assert_ne!` against the crate version alone would happily accept "".
        // Expect a leading agent major version, e.g. "7.83.0-localbuild" or
        // "7.83.0-devel+git.635.e3326d4".
        let (major, rest) = RUNNER_VERSION
            .split_once('.')
            .unwrap_or_else(|| panic!("malformed agent version {RUNNER_VERSION:?}"));
        assert!(
            !major.is_empty() && major.chars().all(|c| c.is_ascii_digit()),
            "agent version {RUNNER_VERSION:?} must start with a numeric major version"
        );
        assert!(
            rest.contains('.'),
            "agent version {RUNNER_VERSION:?} must have major.minor.patch"
        );
    }

    #[test]
    fn ignores_flat_executor_socket_path_key() {
        let yaml = format!("{MIN_YAML}  executor_socket_path: /tmp/wrong.sock\n");
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.executor_socket, PathBuf::from(DEFAULT_EXECUTOR_SOCKET));
    }

    /// `ipc_cert_file_path` defaults to empty in the agent and is unset on
    /// essentially every host, so par-control has to reproduce Go's fallback chain
    /// (`getCertFilepath` in pkg/api/security/cert/cert_getter.go). If it does
    /// not, dispatch fails the executor's required-client-cert handshake while
    /// OPMS polling keeps working — a silent, confusing failure.
    #[test]
    fn resolves_ipc_cert_path_like_the_agent() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        let identity_yaml = "private_action_runner:\n  urn: \"urn:dd:apps:on-prem-runner:us1:42:r\"\n  private_key: \"k\"\n";

        // 1. Explicit setting wins.
        std::fs::write(
            &cfg_path,
            format!("ipc_cert_file_path: /custom/cert.pem\nauth_token_file_path: /tok/auth_token\n{identity_yaml}"),
        )
        .unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, PathBuf::from("/custom/cert.pem"));

        // 2. Else next to auth_token_file_path.
        std::fs::write(
            &cfg_path,
            format!("auth_token_file_path: /tok/auth_token\n{identity_yaml}"),
        )
        .unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, PathBuf::from("/tok/ipc_cert.pem"));

        // 3. Else next to datadog.yaml — the case that applies to a default host.
        std::fs::write(&cfg_path, identity_yaml).unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, dir.path().join("ipc_cert.pem"));

        // An empty string is the agent's own default and must not be taken
        // literally as a path.
        std::fs::write(
            &cfg_path,
            format!("ipc_cert_file_path: \"\"\n{identity_yaml}"),
        )
        .unwrap();
        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.ipc_cert_file, dir.path().join("ipc_cert.pem"));
    }

    #[test]
    fn honors_http_dd_url_override() {
        let yaml = format!("dd_url: \"http://fake-opms:8080\"\n{MIN_YAML}");
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.opms_base_url, "http://fake-opms:8080");
    }

    #[test]
    fn fails_without_identity() {
        assert!(Config::from_yaml_str("site: datadoghq.com\n").is_err());
    }

    #[test]
    fn resolves_identity_from_sibling_file_when_not_inline() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(
            &cfg_path,
            "site: datadoghq.com\nprivate_action_runner:\n  enabled: true\n",
        )
        .unwrap();
        std::fs::write(
            dir.path().join(crate::identity::DEFAULT_IDENTITY_FILE_NAME),
            r#"{"private_key":"enc","urn":"urn:dd:apps:on-prem-runner:us1:7:r7"}"#,
        )
        .unwrap();

        let cfg = Config::try_from_yaml_file(&cfg_path).unwrap().unwrap();
        assert_eq!(cfg.identity.org_id, 7);
        assert_eq!(cfg.identity.runner_id, "r7");
    }

    #[test]
    fn try_load_returns_none_when_no_identity_anywhere() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(&cfg_path, "site: datadoghq.com\n").unwrap();
        assert!(Config::try_from_yaml_file(&cfg_path).unwrap().is_none());
    }

    /// Split mode requires *both* switches. `enabled` alone is the monolithic
    /// runner's own switch, so honoring it by itself would make par-control and
    /// the Go runner dequeue the same OPMS tasks on every existing PAR host.
    #[test]
    fn split_mode_requires_enabled_and_split_enabled() {
        let cases = [
            ("", false),
            ("private_action_runner:\n  enabled: true\n", false),
            ("private_action_runner:\n  split_enabled: true\n", false),
            (
                "private_action_runner:\n  enabled: true\n  split_enabled: true\n",
                true,
            ),
            (
                "private_action_runner:\n  enabled: false\n  split_enabled: true\n",
                false,
            ),
        ];
        for (yaml, want) in cases {
            let gate = LaunchGate::from_yaml_str(yaml).unwrap();
            assert_eq!(gate.split_mode, want, "yaml: {yaml:?}");
        }
    }

    /// The gate must resolve without an identity: it is read before bootstrap so
    /// a not-yet-enrolled host can still decide whether to enroll at all.
    #[test]
    fn gate_resolves_without_identity() {
        let gate = LaunchGate::from_yaml_str("site: datadoghq.com\n").unwrap();
        assert!(!gate.split_mode);
        // Same default as Go's private_action_runner.self_enroll.
        assert!(gate.self_enroll);
    }

    #[test]
    fn parses_agent_log_levels() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("datadog.yaml");

        for (raw, want) in [
            ("debug", log::Level::Debug),
            ("TRACE", log::Level::Trace),
            ("warn", log::Level::Warn),
            ("warning", log::Level::Warn),
            ("error", log::Level::Error),
            // Agent-only levels the `log` crate cannot express.
            ("critical", log::Level::Error),
            ("off", log::Level::Error),
            // Nonsense must not break startup.
            ("not-a-level", log::Level::Info),
        ] {
            std::fs::write(&path, format!("log_level: {raw}\n")).unwrap();
            assert_eq!(log_level_from_yaml_file(&path), want, "log_level: {raw}");
        }

        // Absent key, and an unreadable file, both default to info.
        std::fs::write(&path, "site: datadoghq.com\n").unwrap();
        assert_eq!(log_level_from_yaml_file(&path), log::Level::Info);
        assert_eq!(
            log_level_from_yaml_file(&dir.path().join("missing.yaml")),
            log::Level::Info
        );
    }

    #[test]
    fn gate_honors_explicit_self_enroll_false() {
        let gate =
            LaunchGate::from_yaml_str("private_action_runner:\n  self_enroll: false\n").unwrap();
        assert!(!gate.self_enroll);
    }

    #[test]
    fn overrides_pool_size_and_intervals() {
        let yaml = format!(
            "{MIN_YAML}  task_concurrency: 3\n  idle_timeout_seconds: 120\n  heartbeat_interval_seconds: 5\n"
        );
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.task_concurrency, 3);
        assert_eq!(cfg.idle_timeout, Duration::from_secs(120));
        assert_eq!(cfg.heartbeat_interval, Duration::from_secs(5));
    }
}
