// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Configuration for par-control: the OPMS endpoint, concurrency, socket paths,
//! and timing. Loaded from the agent `datadog.yaml` (only the fields the control
//! plane needs are deserialized) plus the persisted runner identity.

use crate::identity::Identity;
use anyhow::{Context, Result, bail};
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::Duration;

const SKIP_TASK_VERIFICATION_ENV: &str = "DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION";

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

/// The Go runner and enrollment flow always advertise pull mode.
const DEFAULT_MODE: &str = "pull";

/// Interval between OPMS runner health checks. Deliberately not operator-tunable,
/// exactly like Go: it is the `healthCheckInterval` constant (30_000 ms) in
/// `pkg/privateactionrunner/adapters/config/constants.go`. OPMS reasons about
/// runner liveness from this call, so both deployment modes must report at the
/// same rate.
const HEALTH_CHECK_INTERVAL: Duration = Duration::from_secs(30);

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
    /// Runner liveness reporting to OPMS, independent of task flow.
    pub health_check_interval: Duration,
    pub idle_timeout: Duration,
    pub ready_timeout: Duration,
    /// Bounds the initial cold-executor wait for verified RC keys.
    pub key_sync_timeout: Duration,
    pub opms_request_timeout: Duration,
    pub opms_extra_headers: HashMap<String, String>,
    pub proxy: ProxyConfig,
    pub tls: TlsConfig,
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

/// Effective Agent proxy settings used for the OPMS request scheme.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct ProxyConfig {
    pub http: Option<String>,
    pub https: Option<String>,
    pub no_proxy: Vec<String>,
    pub no_proxy_nonexact_match: bool,
}

/// Agent TLS settings used for OPMS connections.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct TlsConfig {
    pub skip_ssl_validation: bool,
    pub min_tls_version: String,
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
    proxy: Option<RawProxy>,
    no_proxy_nonexact_match: Option<bool>,
    skip_ssl_validation: Option<bool>,
    min_tls_version: Option<String>,
    private_action_runner: Option<RawPar>,
}

#[derive(serde::Deserialize, Default, Clone)]
struct RawProxy {
    http: Option<String>,
    https: Option<String>,
    #[serde(default)]
    no_proxy: Vec<String>,
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
    opms_extra_headers: HashMap<String, String>,
}

/// The `private_action_runner.executor.*` sub-section.
#[derive(serde::Deserialize, Default, Clone)]
struct RawParExecutor {
    socket_path: Option<String>,
}

/// Versioned JSON contract emitted by the Go `resolve-control-config` command.
/// Every value has already passed through the Agent's environment precedence,
/// secret backend, Fleet Policy merge, FIPS handling, and config transforms.
#[derive(serde::Deserialize)]
struct EffectiveConfig {
    schema_version: u32,
    main_endpoint: String,
    task_concurrency: usize,
    executor_socket: String,
    procmgr_socket: String,
    executor_process_name: String,
    idle_timeout_seconds: u64,
    heartbeat_interval_seconds: u64,
    #[serde(default)]
    opms_extra_headers: HashMap<String, String>,
    #[serde(default)]
    proxy: EffectiveProxy,
    no_proxy_nonexact_match: bool,
    skip_ssl_validation: bool,
    min_tls_version: String,
    ipc_cert_file: PathBuf,
    #[serde(default)]
    urn: String,
    #[serde(default)]
    private_key: String,
}

#[derive(serde::Deserialize, Default)]
struct EffectiveProxy {
    #[serde(default)]
    http: String,
    #[serde(default)]
    https: String,
    #[serde(default)]
    no_proxy: Vec<String>,
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
        Self::try_from_yaml_file_with_env(path, |name| std::env::var(name).ok())
    }

    /// Ask the existing Go Private Action Runner binary for a snapshot of the
    /// Agent's effective config. Capturing stdout keeps resolved credentials in
    /// memory rather than persisting a second plaintext config file.
    pub fn try_from_agent_config(path: &Path, helper: &Path) -> Result<Option<Self>> {
        let output = Command::new(helper)
            .arg("resolve-control-config")
            .arg("--cfgpath")
            .arg(path)
            .output()
            .with_context(|| {
                format!("failed to run effective-config helper {}", helper.display())
            })?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            bail!(
                "effective-config helper {} exited with {}: {}",
                helper.display(),
                output.status,
                stderr.trim()
            );
        }
        let effective: EffectiveConfig = serde_json::from_slice(&output.stdout)
            .context("failed to parse effective config from Go helper")?;
        Self::try_from_effective(
            effective,
            std::env::var(SKIP_TASK_VERIFICATION_ENV).as_deref() == Ok("true"),
        )
    }

    fn try_from_yaml_file_with_env(
        path: &Path,
        env: impl Fn(&str) -> Option<String>,
    ) -> Result<Option<Self>> {
        let contents = std::fs::read_to_string(path)
            .with_context(|| format!("failed to read config file: {}", path.display()))?;
        let mut raw: RawConfig =
            serde_yaml::from_str(&contents).context("failed to parse datadog.yaml")?;
        apply_env_overrides(&mut raw, &env)?;

        let identity = match inline_identity(raw.private_action_runner.as_ref())? {
            Some(id) => Some(id),
            None => Identity::from_file(&identity_file_path(&raw, path))?,
        };
        match identity {
            Some(identity) => Ok(Some(Self::build(
                raw,
                identity,
                path,
                env(SKIP_TASK_VERIFICATION_ENV).as_deref() == Some("true"),
            )?)),
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
        Self::build(raw, identity, Path::new("datadog.yaml"), false)
    }

    fn build(
        raw: RawConfig,
        identity: Identity,
        config_path: &Path,
        allow_insecure_opms: bool,
    ) -> Result<Self> {
        let par = raw.private_action_runner.clone().unwrap_or_default();
        let opms_base_url = resolve_opms_base_url(
            raw.site.as_deref(),
            raw.dd_url.as_deref(),
            allow_insecure_opms,
        )?;
        let task_concurrency = par.task_concurrency.unwrap_or(5);
        if task_concurrency == 0 {
            bail!("private_action_runner.task_concurrency must be greater than zero");
        }
        let heartbeat_interval = par.heartbeat_interval_seconds.unwrap_or(20);
        if heartbeat_interval == 0 {
            bail!("private_action_runner.heartbeat_interval_seconds must be greater than zero");
        }
        let proxy = effective_proxy(&raw);
        let ipc_cert_file = ipc_cert_file_path(&raw, config_path);

        Ok(Config {
            opms_base_url,
            task_concurrency,
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
            heartbeat_interval: Duration::from_secs(heartbeat_interval),
            health_check_interval: HEALTH_CHECK_INTERVAL,
            idle_timeout: Duration::from_secs(par.idle_timeout_seconds.unwrap_or(60)),
            ready_timeout: Duration::from_secs(10),
            key_sync_timeout: Duration::from_secs(120),
            opms_request_timeout: Duration::from_secs(30),
            opms_extra_headers: par.opms_extra_headers,
            proxy,
            tls: TlsConfig {
                skip_ssl_validation: raw.skip_ssl_validation.unwrap_or(false),
                min_tls_version: raw.min_tls_version.unwrap_or_else(|| "tlsv1.2".to_string()),
            },
            // Backoff defaults mirror pkg/privateactionrunner/adapters/config/constants.go.
            min_backoff: Duration::from_secs(1),
            max_backoff: Duration::from_secs(180),
            wait_before_retry: Duration::from_secs(300),
            max_attempts: 20,
            runner_version: RUNNER_VERSION.to_string(),
            modes: vec![DEFAULT_MODE.to_string()],
            ipc_cert_file,
            identity,
        })
    }

    fn try_from_effective(
        effective: EffectiveConfig,
        allow_insecure_opms: bool,
    ) -> Result<Option<Self>> {
        if effective.schema_version != 1 {
            bail!(
                "unsupported effective-config schema version {}",
                effective.schema_version
            );
        }
        let inline_identity = match (
            (!effective.urn.is_empty()).then_some(effective.urn.as_str()),
            (!effective.private_key.is_empty()).then_some(effective.private_key.as_str()),
        ) {
            (Some(urn), Some(key)) => Some(Identity::new(urn.to_string(), key.to_string())?),
            _ => None,
        };
        let Some(identity) = inline_identity else {
            return Ok(None);
        };
        if effective.task_concurrency == 0 {
            bail!("private_action_runner.task_concurrency must be greater than zero");
        }
        if effective.heartbeat_interval_seconds == 0 {
            bail!("private_action_runner.heartbeat_interval_seconds must be greater than zero");
        }

        Ok(Some(Config {
            opms_base_url: resolve_opms_base_url(
                None,
                Some(&effective.main_endpoint),
                allow_insecure_opms,
            )?,
            task_concurrency: effective.task_concurrency,
            executor_socket: PathBuf::from(effective.executor_socket),
            procmgr_socket: PathBuf::from(effective.procmgr_socket),
            executor_process_name: effective.executor_process_name,
            loop_interval: Duration::from_secs(1),
            heartbeat_interval: Duration::from_secs(effective.heartbeat_interval_seconds),
            health_check_interval: HEALTH_CHECK_INTERVAL,
            idle_timeout: Duration::from_secs(effective.idle_timeout_seconds),
            ready_timeout: Duration::from_secs(10),
            key_sync_timeout: Duration::from_secs(120),
            opms_request_timeout: Duration::from_secs(30),
            opms_extra_headers: effective.opms_extra_headers,
            proxy: ProxyConfig {
                http: (!effective.proxy.http.is_empty()).then_some(effective.proxy.http),
                https: (!effective.proxy.https.is_empty()).then_some(effective.proxy.https),
                no_proxy: effective.proxy.no_proxy,
                no_proxy_nonexact_match: effective.no_proxy_nonexact_match,
            },
            tls: TlsConfig {
                skip_ssl_validation: effective.skip_ssl_validation,
                min_tls_version: effective.min_tls_version,
            },
            min_backoff: Duration::from_secs(1),
            max_backoff: Duration::from_secs(180),
            wait_before_retry: Duration::from_secs(300),
            max_attempts: 20,
            runner_version: RUNNER_VERSION.to_string(),
            modes: vec![DEFAULT_MODE.to_string()],
            ipc_cert_file: effective.ipc_cert_file,
            identity,
        }))
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
    std::env::var("DD_LOG_LEVEL")
        .ok()
        .or_else(|| {
            std::fs::read_to_string(path)
                .ok()
                .and_then(|contents| serde_yaml::from_str::<RawConfig>(&contents).ok())
                .and_then(|raw| raw.log_level)
        })
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
        Self::from_yaml_str_with_env(&contents, |name| std::env::var(name).ok())
    }

    pub fn from_yaml_str(yaml: &str) -> Result<Self> {
        Self::from_yaml_str_with_env(yaml, |_| None)
    }

    fn from_yaml_str_with_env(yaml: &str, env: impl Fn(&str) -> Option<String>) -> Result<Self> {
        let raw: RawConfig = serde_yaml::from_str(yaml).context("failed to parse datadog.yaml")?;
        let par = raw.private_action_runner.unwrap_or_default();
        let enabled = env_bool_override(par.enabled, "DD_PRIVATE_ACTION_RUNNER_ENABLED", &env)?;
        let split_enabled = env_bool_override(
            par.split_enabled,
            "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED",
            &env,
        )?;
        let self_enroll = env_bool_override(
            par.self_enroll,
            "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL",
            &env,
        )?;
        Ok(Self {
            split_mode: enabled.unwrap_or(false) && split_enabled.unwrap_or(false),
            self_enroll: self_enroll.unwrap_or(true),
        })
    }
}

fn env_bool_override(
    yaml_value: Option<bool>,
    name: &str,
    env: &impl Fn(&str) -> Option<String>,
) -> Result<Option<bool>> {
    let Some(raw) = env(name) else {
        return Ok(yaml_value);
    };
    let value = match raw.trim().to_ascii_lowercase().as_str() {
        "1" | "t" | "true" | "y" | "yes" => true,
        "0" | "f" | "false" | "n" | "no" => false,
        _ => bail!("invalid boolean value for {name}: {raw:?}"),
    };
    Ok(Some(value))
}

fn env_override<T>(
    current: Option<T>,
    name: &str,
    env: &impl Fn(&str) -> Option<String>,
) -> Result<Option<T>>
where
    T: std::str::FromStr,
    T::Err: std::fmt::Display,
{
    match env(name) {
        Some(raw) => raw
            .trim()
            .parse()
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {name}: {e}")),
        None => Ok(current),
    }
}

fn first_env(env: &impl Fn(&str) -> Option<String>, names: &[&str]) -> Option<String> {
    names.iter().find_map(|name| env(name))
}

fn apply_env_overrides(raw: &mut RawConfig, env: &impl Fn(&str) -> Option<String>) -> Result<()> {
    raw.site = env("DD_SITE").or(raw.site.take());
    raw.dd_url = env("DD_DD_URL")
        .or_else(|| env("DD_URL"))
        .or(raw.dd_url.take());
    raw.ipc_cert_file_path = env("DD_IPC_CERT_FILE_PATH").or(raw.ipc_cert_file_path.take());
    raw.auth_token_file_path = env("DD_AUTH_TOKEN_FILE_PATH").or(raw.auth_token_file_path.take());
    raw.no_proxy_nonexact_match = env_bool_override(
        raw.no_proxy_nonexact_match,
        "DD_NO_PROXY_NONEXACT_MATCH",
        env,
    )?;

    let proxy = raw.proxy.get_or_insert_with(RawProxy::default);
    proxy.http =
        first_env(env, &["DD_PROXY_HTTP", "HTTP_PROXY", "http_proxy"]).or(proxy.http.take());
    proxy.https =
        first_env(env, &["DD_PROXY_HTTPS", "HTTPS_PROXY", "https_proxy"]).or(proxy.https.take());
    if let Some(no_proxy) = first_env(env, &["DD_PROXY_NO_PROXY", "NO_PROXY", "no_proxy"]) {
        proxy.no_proxy = no_proxy
            .split(|c: char| c == ',' || c.is_ascii_whitespace())
            .filter(|entry| !entry.is_empty())
            .map(str::to_string)
            .collect();
    }

    let par = raw
        .private_action_runner
        .get_or_insert_with(RawPar::default);
    par.enabled = env_bool_override(par.enabled, "DD_PRIVATE_ACTION_RUNNER_ENABLED", env)?;
    par.split_enabled = env_bool_override(
        par.split_enabled,
        "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED",
        env,
    )?;
    par.self_enroll =
        env_bool_override(par.self_enroll, "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL", env)?;
    par.urn = env("DD_PRIVATE_ACTION_RUNNER_URN").or(par.urn.take());
    par.private_key = env("DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY").or(par.private_key.take());
    par.identity_file_path =
        env("DD_PRIVATE_ACTION_RUNNER_IDENTITY_FILE_PATH").or(par.identity_file_path.take());
    par.task_concurrency = env_override(
        par.task_concurrency,
        "DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY",
        env,
    )?;
    par.procmgr_socket_path =
        env("DD_PRIVATE_ACTION_RUNNER_PROCMGR_SOCKET_PATH").or(par.procmgr_socket_path.take());
    par.executor_process_name =
        env("DD_PRIVATE_ACTION_RUNNER_EXECUTOR_PROCESS_NAME").or(par.executor_process_name.take());
    par.idle_timeout_seconds = env_override(
        par.idle_timeout_seconds,
        "DD_PRIVATE_ACTION_RUNNER_IDLE_TIMEOUT_SECONDS",
        env,
    )?;
    par.heartbeat_interval_seconds = env_override(
        par.heartbeat_interval_seconds,
        "DD_PRIVATE_ACTION_RUNNER_HEARTBEAT_INTERVAL_SECONDS",
        env,
    )?;
    if let Some(headers) = env("DD_PRIVATE_ACTION_RUNNER_OPMS_EXTRA_HEADERS") {
        par.opms_extra_headers = serde_json::from_str(&headers).context(
            "DD_PRIVATE_ACTION_RUNNER_OPMS_EXTRA_HEADERS must be a JSON object of string values",
        )?;
    }

    let executor = par.executor.get_or_insert_with(RawParExecutor::default);
    executor.socket_path =
        env("DD_PRIVATE_ACTION_RUNNER_EXECUTOR_SOCKET_PATH").or(executor.socket_path.take());
    Ok(())
}

fn effective_proxy(raw: &RawConfig) -> ProxyConfig {
    let proxy = raw.proxy.clone().unwrap_or_default();
    ProxyConfig {
        http: proxy.http.filter(|value| !value.is_empty()),
        https: proxy.https.filter(|value| !value.is_empty()),
        no_proxy: proxy.no_proxy,
        no_proxy_nonexact_match: raw.no_proxy_nonexact_match.unwrap_or(false),
    }
}

/// Resolve the production OPMS origin, allowing plaintext only for the existing
/// verification-bypass E2E mode. For HTTPS `dd_url`, derive the Datadog site the
/// same way as the Go runner rather than silently falling back to US1.
fn resolve_opms_base_url(
    site: Option<&str>,
    dd_url: Option<&str>,
    allow_insecure_opms: bool,
) -> Result<String> {
    if let Some(raw_url) = dd_url.filter(|url| !url.is_empty()) {
        let url = reqwest::Url::parse(raw_url).context("invalid dd_url")?;
        if url.scheme() == "http" {
            if !allow_insecure_opms {
                bail!("plaintext OPMS requires {SKIP_TASK_VERIFICATION_ENV}=true");
            }
            let host = url.host_str().context("dd_url has no host")?;
            let authority = match url.port() {
                Some(port) => format!("{host}:{port}"),
                None => host.to_string(),
            };
            return Ok(format!("http://{authority}"));
        }
        if url.scheme() != "https" {
            bail!("unsupported dd_url scheme {:?}", url.scheme());
        }
        let host = url.host_str().context("dd_url has no host")?;
        let site = extract_datadog_site(host).with_context(|| {
            format!("cannot derive a Datadog site from HTTPS dd_url {raw_url:?}")
        })?;
        return Ok(format!("https://api.{site}"));
    }

    let site = site.unwrap_or("datadoghq.com").trim();
    if site.is_empty() {
        bail!("site is empty and no dd_url override provided");
    }
    Ok(format!("https://api.{site}"))
}

fn extract_datadog_site(host: &str) -> Option<String> {
    let host = host.trim_end_matches('.').to_ascii_lowercase();
    let labels: Vec<&str> = host.split('.').collect();
    if labels.len() < 2 {
        return None;
    }
    let domain_start = labels.len() - 2;
    let recognized_domain = matches!(
        &labels[domain_start..],
        ["datadoghq" | "datad0g", "com" | "eu"] | ["ddog-gov", "com"]
    );
    if !recognized_domain {
        return None;
    }

    let start = domain_start.checked_sub(1).filter(|index| {
        let dc = labels[*index];
        let letters = dc.bytes().take_while(u8::is_ascii_lowercase).count();
        letters >= 2
            && dc[letters..].len() <= 2
            && !dc[letters..].is_empty()
            && dc[letters..].bytes().all(|byte| byte.is_ascii_digit())
    });
    Some(labels[start.unwrap_or(domain_start)..].join("."))
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
    if let Some(auth_token) = raw
        .auth_token_file_path
        .as_deref()
        .filter(|s| !s.is_empty())
    {
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
        assert_eq!(cfg.modes, vec!["pull"]);
        // Must match the procmgr process-definition name installed by the
        // installer, else Start/Describe/Stop fail against dd-procmgrd.
        assert_eq!(cfg.executor_process_name, "datadog-agent-action-executor");
        assert_eq!(cfg.executor_socket, PathBuf::from(DEFAULT_EXECUTOR_SOCKET));
    }

    /// The executor socket lives under the *nested* `executor:` block, matching the
    /// Go config key `private_action_runner.executor.socket_path`. A flat
    /// `executor_socket_path` key must be ignored, so par-control and the Go
    /// executor can never disagree about which socket to use.
    #[test]
    fn reads_nested_executor_socket_path() {
        let yaml = format!("{MIN_YAML}  executor:\n    socket_path: /tmp/custom-executor.sock\n");
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
        let injected = env!("DD_AGENT_VERSION");
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
    fn plaintext_opms_requires_test_bypass() {
        assert_eq!(
            resolve_opms_base_url(None, Some("http://fake-opms:8080"), true).unwrap(),
            "http://fake-opms:8080"
        );
        assert!(resolve_opms_base_url(None, Some("http://fake-opms:8080"), false).is_err());
    }

    #[test]
    fn https_dd_url_selects_the_matching_datadog_site() {
        assert_eq!(
            resolve_opms_base_url(
                Some("datadoghq.com"),
                Some("https://app.us3.datadoghq.com"),
                false,
            )
            .unwrap(),
            "https://api.us3.datadoghq.com"
        );
        assert_eq!(
            resolve_opms_base_url(None, Some("https://api.datad0g.eu."), false).unwrap(),
            "https://api.datad0g.eu"
        );
        assert!(resolve_opms_base_url(None, Some("https://custom.example.com"), false).is_err());
    }

    #[test]
    fn full_config_honors_environment_overrides() {
        let dir = tempfile::tempdir().unwrap();
        let cfg_path = dir.path().join("datadog.yaml");
        std::fs::write(&cfg_path, MIN_YAML).unwrap();
        let env = |name: &str| match name {
            "DD_SITE" => Some("datadoghq.eu".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_URN" => {
                Some("urn:dd:apps:on-prem-runner:eu1:7:env-runner".to_string())
            }
            "DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY" => Some("env-key".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY" => Some("3".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_EXECUTOR_SOCKET_PATH" => {
                Some("/tmp/env-executor.sock".to_string())
            }
            "DD_PROXY_HTTPS" => Some("http://proxy.example:3128".to_string()),
            "DD_PROXY_NO_PROXY" => Some("localhost, 127.0.0.1".to_string()),
            "DD_NO_PROXY_NONEXACT_MATCH" => Some("true".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_OPMS_EXTRA_HEADERS" => {
                Some(r#"{"X-Test-Routing":"canary"}"#.to_string())
            }
            _ => None,
        };

        let cfg = Config::try_from_yaml_file_with_env(&cfg_path, env)
            .unwrap()
            .unwrap();
        assert_eq!(cfg.opms_base_url, "https://api.datadoghq.eu");
        assert_eq!(cfg.identity.org_id, 7);
        assert_eq!(cfg.identity.runner_id, "env-runner");
        assert_eq!(cfg.identity.private_key, "env-key");
        assert_eq!(cfg.task_concurrency, 3);
        assert_eq!(cfg.executor_socket, PathBuf::from("/tmp/env-executor.sock"));
        assert_eq!(
            cfg.proxy.https.as_deref(),
            Some("http://proxy.example:3128")
        );
        assert_eq!(cfg.proxy.no_proxy, ["localhost", "127.0.0.1"]);
        assert!(cfg.proxy.no_proxy_nonexact_match);
        assert_eq!(
            cfg.opms_extra_headers
                .get("X-Test-Routing")
                .map(String::as_str),
            Some("canary")
        );
    }

    #[test]
    fn rejects_zero_concurrency_and_heartbeat_interval() {
        let concurrency = format!("{MIN_YAML}  task_concurrency: 0\n");
        assert!(Config::from_yaml_str(&concurrency).is_err());

        let heartbeat = format!("{MIN_YAML}  heartbeat_interval_seconds: 0\n");
        assert!(Config::from_yaml_str(&heartbeat).is_err());
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

    #[test]
    fn launch_gate_environment_overrides_yaml() {
        let env = |name: &str| match name {
            "DD_PRIVATE_ACTION_RUNNER_ENABLED" => Some("true".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED" => Some("1".to_string()),
            "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL" => Some("false".to_string()),
            _ => None,
        };
        let gate = LaunchGate::from_yaml_str_with_env(
            "private_action_runner:\n  enabled: false\n  split_enabled: false\n",
            env,
        )
        .unwrap();
        assert!(gate.split_mode);
        assert!(!gate.self_enroll);
    }

    #[test]
    fn launch_gate_rejects_invalid_environment_boolean() {
        let result = LaunchGate::from_yaml_str_with_env("", |name| {
            (name == "DD_PRIVATE_ACTION_RUNNER_ENABLED").then(|| "sometimes".to_string())
        });
        assert!(result.is_err());
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
    fn overrides_pool_size_and_intervals_but_always_uses_pull_mode() {
        let yaml = format!(
            "{MIN_YAML}  task_concurrency: 3\n  idle_timeout_seconds: 120\n  heartbeat_interval_seconds: 5\n  modes: [push]\n"
        );
        let cfg = Config::from_yaml_str(&yaml).unwrap();
        assert_eq!(cfg.task_concurrency, 3);
        assert_eq!(cfg.idle_timeout, Duration::from_secs(120));
        assert_eq!(cfg.heartbeat_interval, Duration::from_secs(5));
        // Liveness reporting is a fixed contract with OPMS, not a knob.
        assert_eq!(cfg.health_check_interval, Duration::from_secs(30));
        // `modes` is intentionally not deserialized. Enrollment and the Go
        // monolith support pull mode only, so an undocumented YAML key cannot
        // make the runtime advertise a different capability.
        assert_eq!(cfg.modes, vec!["pull"]);
    }

    #[test]
    fn consumes_effective_config_from_go_contract() {
        let effective: EffectiveConfig = serde_json::from_str(
            r#"{
                "schema_version": 1,
                "main_endpoint": "https://app.us3.datadoghq.com",
                "task_concurrency": 4,
                "executor_socket": "/resolved/executor.sock",
                "procmgr_socket": "/resolved/procmgr.sock",
                "executor_process_name": "resolved-executor",
                "idle_timeout_seconds": 90,
                "heartbeat_interval_seconds": 15,
                "opms_extra_headers": {"X-Resolved": "secret-value"},
                "proxy": {
                    "https": "http://resolved-user:resolved-pass@proxy:3128",
                    "no_proxy": ["localhost"]
                },
                "no_proxy_nonexact_match": true,
                "skip_ssl_validation": true,
                "min_tls_version": "tlsv1.3",
                "ipc_cert_file": "/resolved/ipc.pem",
                "urn": "urn:dd:apps:on-prem-runner:us3:42:resolved-runner",
                "private_key": "resolved-private-key"
            }"#,
        )
        .unwrap();

        let cfg = Config::try_from_effective(effective, false)
            .unwrap()
            .unwrap();
        assert_eq!(cfg.opms_base_url, "https://api.us3.datadoghq.com");
        assert_eq!(cfg.task_concurrency, 4);
        assert_eq!(
            cfg.executor_socket,
            PathBuf::from("/resolved/executor.sock")
        );
        assert_eq!(
            cfg.proxy.https.as_deref(),
            Some("http://resolved-user:resolved-pass@proxy:3128")
        );
        assert_eq!(cfg.proxy.no_proxy, ["localhost"]);
        assert!(cfg.proxy.no_proxy_nonexact_match);
        assert!(cfg.tls.skip_ssl_validation);
        assert_eq!(cfg.tls.min_tls_version, "tlsv1.3");
        assert_eq!(cfg.modes, ["pull"]);
        assert_eq!(cfg.health_check_interval, Duration::from_secs(30));
        assert_eq!(cfg.identity.private_key, "resolved-private-key");
    }
}
