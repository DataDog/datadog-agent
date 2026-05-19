use std::fs;
use std::path::{Path, PathBuf};
use std::time::Duration;

use chrono::Utc;
use serde::Deserialize;
use serde::Serialize;
use serde_json::Value;

const CONFIG_BASENAME: &str = "ai_usage_native_host.yaml";
const AI_USAGE_EVP_SUBDOMAIN: &str = "event-platform-intake";
const AI_USAGE_EVP_PATH: &str = "/api/v2/aiusage";

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
    /// - `trace_agent_url` (default `http://localhost:8126`; use **http** only — the local trace
    ///   receiver is plain HTTP and this binary has no TLS client)
    /// - `evp_proxy_api_version` (default `2`)
    /// - `ai_usage_evp_subdomain` (default `event-platform-intake`)
    ///
    /// No `DD_API_KEY` is required here; the Agent injects the key when forwarding.
    pub fn load(config_path: Option<PathBuf>) -> Self {
        let mut agent_base = "http://localhost:8126".to_string();
        let mut proxy_version: u32 = 2;
        let mut evp_subdomain = AI_USAGE_EVP_SUBDOMAIN.to_string();

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
        let intake_url = format!("{}/evp_proxy/v{}{}", base, proxy_version, AI_USAGE_EVP_PATH);

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
        if path.is_file() {
            Some(path)
        } else {
            None
        }
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
        use std::ffi::OsStr;
        use std::os::windows::ffi::OsStrExt;
        use std::ptr;
        use windows_sys::Win32::Foundation::ERROR_SUCCESS;
        use windows_sys::Win32::System::Environment::ExpandEnvironmentStringsW;
        use windows_sys::Win32::System::Registry::{
            RegCloseKey, RegOpenKeyExW, RegQueryValueExW, HKEY, HKEY_LOCAL_MACHINE, KEY_READ,
            KEY_WOW64_64KEY, REG_EXPAND_SZ, REG_SZ,
        };

        fn wide(value: &str) -> Vec<u16> {
            OsStr::new(value).encode_wide().chain(Some(0)).collect()
        }

        unsafe fn expand_env_string(value: &[u16]) -> Option<Vec<u16>> {
            let mut source = value.to_vec();
            source.push(0);

            let required =
                unsafe { ExpandEnvironmentStringsW(source.as_ptr(), ptr::null_mut(), 0) };
            if required == 0 {
                return None;
            }

            let mut expanded = vec![0u16; required as usize];
            let written = unsafe {
                ExpandEnvironmentStringsW(source.as_ptr(), expanded.as_mut_ptr(), required)
            };
            if written == 0 || written > required {
                return None;
            }

            let len = expanded
                .iter()
                .position(|c| *c == 0)
                .unwrap_or(expanded.len());
            if len == 0 {
                return None;
            }
            expanded.truncate(len);
            Some(expanded)
        }

        unsafe {
            let mut key: HKEY = ptr::null_mut();
            let subkey = wide(subkey);
            let status = RegOpenKeyExW(
                HKEY_LOCAL_MACHINE,
                subkey.as_ptr(),
                0,
                KEY_READ | KEY_WOW64_64KEY,
                &mut key,
            );
            if status != ERROR_SUCCESS {
                return None;
            }

            let value_name = wide(value_name);
            let mut value_type = 0;
            let mut byte_len = 0;
            let status = RegQueryValueExW(
                key,
                value_name.as_ptr(),
                ptr::null_mut(),
                &mut value_type,
                ptr::null_mut(),
                &mut byte_len,
            );
            if status != ERROR_SUCCESS
                || byte_len < 2
                || (value_type != REG_SZ && value_type != REG_EXPAND_SZ)
            {
                RegCloseKey(key);
                return None;
            }

            let mut buffer = vec![0u16; (byte_len as usize).div_ceil(2)];
            let status = RegQueryValueExW(
                key,
                value_name.as_ptr(),
                ptr::null_mut(),
                &mut value_type,
                buffer.as_mut_ptr().cast(),
                &mut byte_len,
            );
            RegCloseKey(key);
            if status != ERROR_SUCCESS {
                return None;
            }

            let len = buffer.iter().position(|c| *c == 0).unwrap_or(buffer.len());
            if len == 0 {
                return None;
            }
            let value = if value_type == REG_EXPAND_SZ {
                expand_env_string(&buffer[..len])?
            } else {
                buffer[..len].to_vec()
            };
            Some(PathBuf::from(String::from_utf16_lossy(&value)))
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
            Err(e) => {
                eprintln!("[datadog] failed to build AI usage payload: {}", e);
                return false;
            }
        };

        match ureq::post(&self.intake_url)
            .timeout(AGENT_REQUEST_TIMEOUT)
            .set("Content-Type", "application/json")
            .set("X-Datadog-EVP-Subdomain", &self.evp_subdomain)
            .send_json(&body)
        {
            Ok(resp) => {
                eprintln!(
                    "[datadog] AI usage event shipped via agent ({})",
                    resp.status()
                );
                true
            }
            Err(e) => {
                eprintln!("[datadog] failed to ship AI usage event via agent: {}", e);
                false
            }
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
}
