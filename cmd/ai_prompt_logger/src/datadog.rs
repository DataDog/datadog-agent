use std::fs;
use std::path::{Path, PathBuf};
use std::time::Duration;

use chrono::Utc;
use serde::Deserialize;
use serde::Serialize;
use serde_json::Value;

use crate::desktop;

const CONFIG_BASENAME: &str = "ai_usage_native_host.yaml";
const AI_USAGE_EVP_SUBDOMAIN: &str = "softinv-intake";
const AI_USAGE_EVP_PATH: &str = "/api/v2/aiusage";
const DEFAULT_CONFIG_YAML: &str = include_str!("../ai_usage_native_host.yaml.example");

/// Cap for connect + full request so the native host thread cannot block indefinitely
/// on a stalled trace Agent / network path.
const AGENT_REQUEST_TIMEOUT: Duration = Duration::from_secs(10);

/// Ships AI usage events via the local Agent trace receiver's EVP proxy
/// (`/evp_proxy/v*{AI_USAGE_EVP_PATH}`). The Agent adds `DD-API-KEY` and
/// forwards to `https://{subdomain}.{site}{AI_USAGE_EVP_PATH}`.
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
    ai_usage_evp_subdomain: Option<String>,
    #[serde(default)]
    desktop_monitoring: Option<DesktopMonitoringConfig>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct AiProcessConfig {
    pub process_names: Vec<String>,
    pub tool: String,
    pub provider: String,
    #[serde(default)]
    pub match_scope: AiProcessMatchScope,
    #[serde(default)]
    pub window_title_hints: Vec<String>,
    #[serde(default)]
    pub approved: bool,
    #[serde(default)]
    pub secondary: bool,
}

#[derive(Debug, Clone, Copy, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum AiProcessMatchScope {
    Direct,
    HostedChild,
    Both,
}

impl Default for AiProcessMatchScope {
    fn default() -> Self {
        Self::Both
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct DesktopMonitoringConfig {
    #[serde(default = "default_desktop_monitoring_enabled")]
    pub enabled: bool,
    #[serde(default)]
    pub debug: u8,
    #[serde(default = "default_desktop_monitoring_poll_interval_seconds")]
    pub poll_interval_seconds: u64,
    #[serde(default = "default_ai_process_names")]
    pub ai_process_names: Vec<AiProcessConfig>,
    #[serde(default = "default_host_process_names")]
    pub host_process_names: Vec<String>,
}

impl Default for DesktopMonitoringConfig {
    fn default() -> Self {
        default_desktop_monitoring_from_embedded_yaml()
            .unwrap_or_else(disabled_desktop_monitoring_config)
    }
}

fn default_desktop_monitoring_enabled() -> bool {
    true
}

fn default_desktop_monitoring_poll_interval_seconds() -> u64 {
    60
}

fn default_ai_process_names() -> Vec<AiProcessConfig> {
    default_desktop_monitoring_from_embedded_yaml()
        .map(|config| config.ai_process_names)
        .unwrap_or_default()
}

fn default_host_process_names() -> Vec<String> {
    default_desktop_monitoring_from_embedded_yaml()
        .map(|config| config.host_process_names)
        .unwrap_or_default()
}

fn default_desktop_monitoring_from_embedded_yaml() -> Option<DesktopMonitoringConfig> {
    match serde_yaml::from_str::<AiUsageNativeHostFile>(DEFAULT_CONFIG_YAML) {
        Ok(file) => file.desktop_monitoring,
        Err(err) => {
            desktop::log_startup_warning(format!(
                "embedded AI usage config YAML is invalid: {err}"
            ));
            None
        }
    }
}

fn disabled_desktop_monitoring_config() -> DesktopMonitoringConfig {
    DesktopMonitoringConfig {
        enabled: false,
        debug: 0,
        poll_interval_seconds: default_desktop_monitoring_poll_interval_seconds(),
        ai_process_names: Vec::new(),
        host_process_names: Vec::new(),
    }
}

fn log_desktop_monitoring_config_error(message: impl AsRef<str>) {
    let message = message.as_ref();
    desktop::log_startup_warning(format!("desktop monitoring config error: {message}"));
}

impl DatadogClient {
    /// Load settings from YAML, same style as `agent run -c` / `system-probe --config`.
    ///
    /// If `config_path` is `Some` and points to a file, that file is read. If it is `Some` but
    /// missing or not a file, a warning is logged and defaults are used (no fallback to
    /// auto-discovery for that explicit path).
    ///
    /// If `config_path` is `None`, searches for `CONFIG_BASENAME`:
    /// 1. On Windows, under the MSI `ConfigRoot` registry value.
    /// 2. On Windows, under `%ProgramData%\Datadog`.
    /// 3. Under the install prefix inferred from the executable path.
    ///
    /// YAML keys (defaults match the Agent trace receiver):
    /// - `trace_agent_url` (default `http://127.0.0.1:8126`; use **http** only — the local trace
    ///   receiver is plain HTTP and this binary has no TLS client)
    /// - `evp_proxy_api_version` (default `2`)
    /// - `ai_usage_evp_subdomain` (default `softinv-intake`)
    ///
    /// No `DD_API_KEY` is required here; the Agent injects the key when forwarding.
    pub fn load(config_path: Option<PathBuf>) -> Self {
        let mut agent_base = "http://127.0.0.1:8126".to_string();
        let mut proxy_version: u32 = 2;
        let mut evp_subdomain = AI_USAGE_EVP_SUBDOMAIN.to_string();

        let yaml_path: Option<PathBuf> = if let Some(ref p) = config_path {
            if p.is_file() { Some(p.clone()) } else { None }
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
            }
        }

        let base = agent_base.trim_end_matches('/').to_string();
        let intake_url = format!("{}/evp_proxy/v{}{}", base, proxy_version, AI_USAGE_EVP_PATH);

        Self {
            intake_url,
            evp_subdomain,
        }
    }

    pub fn load_desktop_monitoring_config(config_path: Option<PathBuf>) -> DesktopMonitoringConfig {
        let yaml_path: Option<PathBuf> = if let Some(ref p) = config_path {
            if p.is_file() {
                Some(p.clone())
            } else {
                log_desktop_monitoring_config_error(format!(
                    "--config path is not a readable file: {}",
                    p.display()
                ));
                return disabled_desktop_monitoring_config();
            }
        } else {
            Self::yaml_config_path()
        };

        if let Some(ref yaml_path) = yaml_path {
            match fs::read_to_string(yaml_path) {
                Ok(contents) => {
                    if let Ok(cfg) = serde_yaml::from_str::<AiUsageNativeHostFile>(&contents) {
                        return cfg
                            .desktop_monitoring
                            .unwrap_or_else(disabled_desktop_monitoring_config);
                    }
                    log_desktop_monitoring_config_error(format!(
                        "could not parse desktop monitoring config: {}",
                        yaml_path.display()
                    ));
                    return disabled_desktop_monitoring_config();
                }
                Err(_) => {
                    log_desktop_monitoring_config_error(format!(
                        "could not read config file: {}",
                        yaml_path.display()
                    ));
                    return disabled_desktop_monitoring_config();
                }
            }
        }

        DesktopMonitoringConfig::default()
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
            if let Some(v) = cfg.ai_usage_evp_subdomain.filter(|s| !s.is_empty()) {
                *evp_subdomain = v;
            }
        }
    }

    fn yaml_config_path() -> Option<PathBuf> {
        #[cfg(windows)]
        {
            if let Some(path) = Self::windows_config_path_from_registry() {
                return Some(path);
            }

            std::env::var_os("ProgramData").and_then(|program_data| {
                Self::config_path_in_dir(PathBuf::from(program_data).join("Datadog"))
            })
        }

        #[cfg(not(windows))]
        Self::install_root_config_path()
    }

    fn config_path_in_dir(dir: impl AsRef<Path>) -> Option<PathBuf> {
        let path = dir.as_ref().join(CONFIG_BASENAME);
        if path.is_file() { Some(path) } else { None }
    }

    #[cfg(windows)]
    fn windows_config_path_from_registry() -> Option<PathBuf> {
        // Mirror pkg/util/winutil.GetProgramDataDir() and pkg/util/defaultpaths:
        // prefer the MSI ConfigRoot registry value over the default ProgramData path.
        let config_root =
            Self::windows_registry_string("SOFTWARE\\Datadog\\Datadog Agent", "ConfigRoot")?;
        Some(config_root.join(CONFIG_BASENAME))
    }

    #[cfg(windows)]
    fn windows_registry_string(subkey: &str, value_name: &str) -> Option<PathBuf> {
        use windows_registry::LOCAL_MACHINE;
        use windows_sys::Win32::System::Registry::KEY_WOW64_64KEY;

        let key = LOCAL_MACHINE
            .options()
            .read()
            .access(KEY_WOW64_64KEY)
            .open(subkey)
            .ok()?;

        let value = key.get_string(value_name).ok()?;
        if value.is_empty() {
            None
        } else {
            Some(PathBuf::from(value))
        }
    }

    #[cfg(not(windows))]
    fn install_root_config_path() -> Option<PathBuf> {
        let install_root = Self::install_root_from_exe()?;
        let etc_dd = install_root
            .join("etc")
            .join("datadog-agent")
            .join(CONFIG_BASENAME);
        if etc_dd.is_file() {
            return Some(etc_dd);
        }
        Self::config_path_in_dir(install_root.join("etc"))
    }

    /// Finds `{install_dir}` for packaged non-Windows layouts like
    /// `{install_dir}/embedded/bin/<name>`.
    #[cfg(not(windows))]
    fn install_root_from_exe() -> Option<PathBuf> {
        let exe = std::env::current_exe().ok()?;
        let bin_dir = exe.parent()?;
        // .../embedded/bin -> .../embedded -> install root
        bin_dir.parent()?.parent().map(Path::to_path_buf)
    }

    /// Post one AI usage event as a JSON object.
    /// Returns true only when the payload is successfully accepted by the Agent.
    pub fn send_event(&self, payload: &AiUsageEvent) -> bool {
        let body = match Self::ai_usage_body(payload) {
            Ok(v) => v,
            Err(_) => return false,
        };

        match ureq::post(&self.intake_url)
            .config()
            .timeout_global(Some(AGENT_REQUEST_TIMEOUT))
            .build()
            .header("Content-Type", "application/json")
            .header("X-Datadog-EVP-Subdomain", &self.evp_subdomain)
            .send_json(&body)
        {
            Ok(_) => true,
            Err(_) => false,
        }
    }

    /// Build `{ ... }` for POST /api/v2/aiusage.
    fn ai_usage_body(payload: &AiUsageEvent) -> Result<Value, serde_json::Error> {
        let map = match serde_json::to_value(payload)? {
            Value::Object(m) => m,
            _ => {
                return Err(<serde_json::Error as serde::de::Error>::custom(
                    "AiUsageEvent did not serialize to a JSON object",
                ));
            }
        };

        Ok(Value::Object(map))
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
        Self::new_with_source(
            detection_type,
            "browser_extension",
            tool,
            user_id,
            hostname,
            approved,
        )
    }

    pub fn new_with_source(
        detection_type: &str,
        source: &str,
        tool: String,
        user_id: String,
        hostname: String,
        approved: bool,
    ) -> Self {
        Self {
            event_type: "ai_usage".to_string(),
            timestamp: Utc::now().to_rfc3339(),
            detection_type: detection_type.to_string(),
            source: source.to_string(),
            tool,
            user_id,
            hostname,
            approved,
            provider: None,
        }
    }
}

/// Hostname for the event payload: OS hostname of the machine running the native host.
pub fn resolve_hostname() -> String {
    hostname::get()
        .ok()
        .and_then(|h| h.into_string().ok())
        .unwrap_or_else(|| "unknown".to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ai_usage_body_keeps_event_fields_at_top_level() {
        let mut event = AiUsageEvent::new(
            "observed",
            "gemini".to_string(),
            "user@example.com".to_string(),
            "host-1".to_string(),
            true,
        );
        event.provider = Some("Google".to_string());

        let body = DatadogClient::ai_usage_body(&event).expect("AI usage body should serialize");
        let body = body
            .as_object()
            .expect("AI usage body should be a JSON object");

        assert_eq!(
            body.get("hostname"),
            Some(&Value::String("host-1".to_string()))
        );
        assert_eq!(body.get("tool"), Some(&Value::String("gemini".to_string())));
        assert_eq!(
            body.get("provider"),
            Some(&Value::String("Google".to_string()))
        );
        assert_eq!(body.get("approved"), Some(&Value::Bool(true)));
        assert!(!body.contains_key("message"));
        assert!(!body.contains_key("ddsource"));
        assert!(!body.contains_key("service"));
        assert!(!body.contains_key("status"));
        assert!(!body.contains_key("date"));
    }

    #[test]
    fn desktop_monitoring_debug_defaults_to_off() {
        let cfg = DesktopMonitoringConfig::default();

        assert_eq!(cfg.debug, 0);
    }

    #[test]
    fn desktop_monitoring_defaults_are_loaded_from_embedded_yaml() {
        let cfg = DesktopMonitoringConfig::default();

        assert!(
            cfg.ai_process_names
                .iter()
                .any(|process| process.tool == "Visual Studio Code")
        );
        assert!(
            cfg.host_process_names
                .iter()
                .any(|process| process == "Code")
        );
    }

    #[test]
    fn invalid_desktop_monitoring_yaml_disables_monitoring() {
        let path = std::env::temp_dir().join(format!(
            "ai_usage_native_host_invalid_{}.yaml",
            std::process::id()
        ));
        std::fs::write(&path, "desktop_monitoring: [").expect("invalid YAML should be written");

        let cfg = DatadogClient::load_desktop_monitoring_config(Some(path.clone()));
        let _ = std::fs::remove_file(path);

        assert!(!cfg.enabled);
        assert!(cfg.ai_process_names.is_empty());
        assert!(cfg.host_process_names.is_empty());
    }

    #[test]
    fn desktop_monitoring_debug_level_can_be_configured_from_yaml() {
        let cfg: AiUsageNativeHostFile = serde_yaml::from_str(
            r#"
desktop_monitoring:
  debug: 2
"#,
        )
        .expect("desktop monitoring config should parse");

        assert_eq!(
            cfg.desktop_monitoring
                .expect("desktop monitoring should be present")
                .debug,
            2
        );
    }
}
