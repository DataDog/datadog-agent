use std::fs;
use std::path::{Path, PathBuf};

use chrono::Utc;
use serde::Deserialize;
use serde::Serialize;
use serde_json::{Map, Value, json};

const CONFIG_BASENAME: &str = "ai_usage_native_host.yaml";

/// Ships AI usage events to Datadog Logs via the local Agent trace receiver's
/// EVP proxy (`/evp_proxy/v*/api/v2/logs`). The Agent adds `DD-API-KEY` and
/// forwards to `https://{subdomain}.{site}/api/v2/logs`.
pub struct DatadogClient {
    intake_url: String,
    evp_subdomain: String,
}

#[derive(Debug, Default, Deserialize)]
struct AiUsageNativeHostFile {
    #[serde(default)]
    trace_agent_url: Option<String>,
    #[serde(default)]
    evp_proxy_api_version: Option<u32>,
    #[serde(default)]
    logs_evp_subdomain: Option<String>,
}

impl DatadogClient {
    /// Load settings from YAML, same style as `agent run -c` / `system-probe --config`.
    ///
    /// If `config_path` is `Some` and points to a file, that file is read. If it is `Some` but
    /// missing or not a file, a warning is logged and defaults are used (no fallback to
    /// auto-discovery for that explicit path).
    ///
    /// If `config_path` is `None`, searches for `CONFIG_BASENAME` under the install prefix
    /// inferred from the executable path (`{install_root}/embedded/bin/...`):
    /// 1. `{install_root}/etc/datadog-agent/`
    /// 2. `{install_root}/etc/`
    ///
    /// YAML keys (defaults match the Agent trace receiver):
    /// - `trace_agent_url` (default `http://localhost:8126`)
    /// - `evp_proxy_api_version` (default `2`)
    /// - `logs_evp_subdomain` (default `http-intake.logs`)
    ///
    /// No `DD_API_KEY` is required here; the Agent injects the key when forwarding.
    pub fn load(config_path: Option<PathBuf>) -> Self {
        let mut agent_base = "http://localhost:8126".to_string();
        let mut proxy_version: u32 = 2;
        let mut evp_subdomain = "http-intake.logs".to_string();

        let yaml_path: Option<PathBuf> = if let Some(ref p) = config_path {
            if p.is_file() {
                Some(p.clone())
            } else {
                eprintln!(
                    "[datadog] warning: --config path is not a readable file: {}",
                    p.display()
                );
                None
            }
        } else {
            Self::yaml_config_path()
        };

        if let Some(ref yaml_path) = yaml_path {
            if let Ok(contents) = fs::read_to_string(yaml_path) {
                Self::apply_yaml(
                    &contents,
                    &mut agent_base,
                    &mut proxy_version,
                    &mut evp_subdomain,
                );
            } else {
                eprintln!(
                    "[datadog] warning: could not read config file: {}",
                    yaml_path.display()
                );
            }
        }

        let base = agent_base.trim_end_matches('/').to_string();
        let intake_url = format!("{}/evp_proxy/v{}/api/v2/logs", base, proxy_version);

        eprintln!(
            "[datadog] client initialised, agent_proxy_url={}, evp_subdomain={}",
            intake_url, evp_subdomain
        );

        Self {
            intake_url,
            evp_subdomain,
        }
    }

    fn apply_yaml(
        contents: &str,
        agent_base: &mut String,
        proxy_version: &mut u32,
        evp_subdomain: &mut String,
    ) {
        if let Ok(cfg) = serde_yaml::from_str::<AiUsageNativeHostFile>(contents) {
            if let Some(v) = cfg.trace_agent_url.filter(|s| !s.is_empty()) {
                *agent_base = v;
            }
            if let Some(v) = cfg.evp_proxy_api_version {
                *proxy_version = v;
            }
            if let Some(v) = cfg.logs_evp_subdomain.filter(|s| !s.is_empty()) {
                *evp_subdomain = v;
            }
        }
    }

    fn yaml_config_path() -> Option<PathBuf> {
        let install_root = Self::install_root_from_exe()?;
        let etc_dd = install_root
            .join("etc")
            .join("datadog-agent")
            .join(CONFIG_BASENAME);
        if etc_dd.is_file() {
            return Some(etc_dd);
        }
        let etc_flat = install_root.join("etc").join(CONFIG_BASENAME);
        if etc_flat.is_file() {
            return Some(etc_flat);
        }
        None
    }

    /// `{install_dir}` when the binary lives at `{install_dir}/embedded/bin/<name>`.
    fn install_root_from_exe() -> Option<PathBuf> {
        let exe = std::env::current_exe().ok()?;
        let bin_dir = exe.parent()?;
        // .../embedded/bin -> .../embedded -> install root
        bin_dir.parent()?.parent().map(Path::to_path_buf)
    }

    /// Post one AI usage event as a single-element Logs v2 JSON array.
    /// Returns true only when the payload is successfully accepted by the Agent.
    pub fn send_event(&self, payload: &AiUsageEvent) -> bool {
        let body = match Self::logs_v2_body(payload) {
            Ok(v) => v,
            Err(e) => {
                eprintln!("[datadog] failed to build log payload: {}", e);
                return false;
            }
        };

        match ureq::post(&self.intake_url)
            .set("Content-Type", "application/json")
            .set("X-Datadog-EVP-Subdomain", &self.evp_subdomain)
            .send_json(&body)
        {
            Ok(resp) => {
                eprintln!("[datadog] log shipped via agent ({})", resp.status());
                true
            }
            Err(e) => {
                eprintln!("[datadog] failed to ship log via agent: {}", e);
                false
            }
        }
    }

    /// Build `[{ ... }]` for POST /api/v2/logs: RFC fields plus Logs envelope.
    fn logs_v2_body(payload: &AiUsageEvent) -> Result<Value, serde_json::Error> {
        let mut map: Map<String, Value> = match serde_json::to_value(payload)? {
            Value::Object(m) => m,
            _ => {
                return Err(<serde_json::Error as serde::de::Error>::custom(
                    "AiUsageEvent did not serialize to a JSON object",
                ));
            }
        };

        map.insert("message".into(), json!("ai_usage"));
        map.insert("ddsource".into(), json!("ai-prompt-logger-native-host"));
        map.insert("service".into(), json!("ai-usage-extension"));
        map.insert("status".into(), json!("info"));
        map.insert("date".into(), json!(Utc::now().timestamp_millis()));

        Ok(json!([Value::Object(map)]))
    }
}

/// RFC-aligned AI usage event payload (embedded as log attributes).
/// `detection_type` discriminates: "intercepted" (deep) vs "observed" (basic).
/// Fields only relevant to deep interception are `Option` and omitted from
/// serialization when `None`, so observed events carry only the minimal fields.
#[derive(Debug, Serialize)]
pub struct AiUsageEvent {
    pub event_type: String,
    pub timestamp: String,
    pub detection_type: String,
    pub source: String,
    pub tool: String,
    pub user_id: String,
    pub hostname: String,
    pub approved: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sentiment: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub category: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sources_accessed: Option<Vec<String>>,
}

impl AiUsageEvent {
    /// Create a new event with the fixed fields pre-populated.
    pub fn new(
        detection_type: &str,
        tool: String,
        user_id: String,
        hostname: String,
        approved: bool,
    ) -> Self {
        Self {
            event_type: "ai_usage".to_string(),
            timestamp: Utc::now().to_rfc3339(),
            detection_type: detection_type.to_string(),
            source: "browser_extension".to_string(),
            tool,
            user_id,
            hostname,
            approved,
            provider: None,
            sentiment: None,
            category: None,
            sources_accessed: None,
        }
    }
}

/// Resolve the hostname for the event payload.
/// Prefers an extension-provided override (from managed storage) when present,
/// falls back to the OS hostname via libc syscall.
pub fn resolve_hostname(override_hostname: Option<&str>) -> String {
    if let Some(h) = override_hostname {
        if !h.is_empty() {
            return h.to_string();
        }
    }
    hostname::get()
        .ok()
        .and_then(|h| h.into_string().ok())
        .unwrap_or_else(|| "unknown".to_string())
}
