use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::PathBuf;
use std::sync::{Mutex, OnceLock};

use serde::Deserialize;

use crate::datadog::{AiProcessConfig, AiProcessMatchScope};
use crate::desktop::log_startup_warning;

#[derive(Debug, Clone)]
pub struct DesktopMonitoringConfig {
    pub enabled: bool,
    pub debug: u8,
    pub poll_interval_seconds: u64,
    pub process_activity_window_seconds: u64,
    pub ai_process_names: Vec<AiProcessConfig>,
    pub host_process_names: Vec<String>,
}

#[derive(Debug, Default, Deserialize)]
struct AiUsageDesktopMonitoringFile {
    #[serde(default)]
    desktop_monitoring: Option<DesktopMonitoringConfigFile>,
}

#[derive(Debug, Default, Deserialize)]
struct DesktopMonitoringConfigFile {
    enabled: Option<bool>,
    debug: Option<u8>,
    poll_interval_seconds: Option<u64>,
    #[serde(alias = "terminal_activity_window_seconds")]
    process_activity_window_seconds: Option<u64>,
    ai_process_names: Option<Vec<AiProcessConfig>>,
    host_process_names: Option<Vec<String>>,
}

impl Default for DesktopMonitoringConfig {
    fn default() -> Self {
        Self {
            enabled: default_desktop_monitoring_enabled(),
            debug: 0,
            poll_interval_seconds: default_desktop_monitoring_poll_interval_seconds(),
            process_activity_window_seconds: default_process_activity_window_seconds(),
            ai_process_names: builtin_ai_process_names(),
            host_process_names: builtin_host_process_names(),
        }
    }
}

fn default_desktop_monitoring_enabled() -> bool {
    true
}

fn default_desktop_monitoring_poll_interval_seconds() -> u64 {
    60
}

fn default_process_activity_window_seconds() -> u64 {
    600
}

impl DesktopMonitoringConfigFile {
    fn merge_with_defaults(self) -> DesktopMonitoringConfig {
        let mut config = DesktopMonitoringConfig::default();
        if let Some(enabled) = self.enabled {
            config.enabled = enabled;
        }
        if let Some(debug) = self.debug {
            config.debug = debug;
        }
        if let Some(poll_interval_seconds) = self.poll_interval_seconds {
            config.poll_interval_seconds = poll_interval_seconds;
        }
        if let Some(process_activity_window_seconds) = self.process_activity_window_seconds {
            config.process_activity_window_seconds = process_activity_window_seconds;
        }
        if let Some(ai_process_names) = self.ai_process_names {
            config.ai_process_names =
                merge_ai_process_names(config.ai_process_names, ai_process_names);
        }
        if let Some(host_process_names) = self.host_process_names {
            config.host_process_names =
                merge_host_process_names(config.host_process_names, host_process_names);
        }
        config
    }
}

fn log_desktop_monitoring_config_error(message: impl AsRef<str>) {
    let message = message.as_ref();
    log_startup_warning(format!("desktop monitoring config error: {message}"));
}

static LAST_GOOD_DESKTOP_MONITORING_CONFIG: OnceLock<Mutex<DesktopMonitoringConfig>> =
    OnceLock::new();
static LAST_DESKTOP_MONITORING_CONFIG_PATH: OnceLock<Mutex<Option<PathBuf>>> = OnceLock::new();

fn desktop_monitoring_config_cache() -> &'static Mutex<DesktopMonitoringConfig> {
    LAST_GOOD_DESKTOP_MONITORING_CONFIG
        .get_or_init(|| Mutex::new(DesktopMonitoringConfig::default()))
}

fn desktop_monitoring_config_path_cache() -> &'static Mutex<Option<PathBuf>> {
    LAST_DESKTOP_MONITORING_CONFIG_PATH.get_or_init(|| Mutex::new(None))
}

fn remember_desktop_monitoring_config_path(config_path: Option<PathBuf>) {
    if let Ok(mut last_path) = desktop_monitoring_config_path_cache().lock() {
        *last_path = config_path;
    }
}

fn last_desktop_monitoring_config_path() -> Option<PathBuf> {
    desktop_monitoring_config_path_cache()
        .lock()
        .ok()
        .and_then(|path| path.clone())
}

fn remember_desktop_monitoring_config(config: DesktopMonitoringConfig) -> DesktopMonitoringConfig {
    if let Ok(mut last_good_config) = desktop_monitoring_config_cache().lock() {
        *last_good_config = config.clone();
    }
    config
}

fn last_good_desktop_monitoring_config() -> DesktopMonitoringConfig {
    desktop_monitoring_config_cache()
        .lock()
        .map(|config| config.clone())
        .unwrap_or_else(|_| DesktopMonitoringConfig::default())
}

#[cfg(test)]
fn reset_desktop_monitoring_config_cache() {
    if let Some(cache) = LAST_GOOD_DESKTOP_MONITORING_CONFIG.get()
        && let Ok(mut config) = cache.lock()
    {
        *config = DesktopMonitoringConfig::default();
    }
    if let Some(cache) = LAST_DESKTOP_MONITORING_CONFIG_PATH.get()
        && let Ok(mut path) = cache.lock()
    {
        *path = None;
    }
}

pub(crate) fn load_desktop_monitoring_config(
    config_path: Option<PathBuf>,
    discover_config_path: impl FnOnce() -> Option<PathBuf>,
) -> DesktopMonitoringConfig {
    let yaml_path: Option<PathBuf> = if let Some(ref p) = config_path {
        if p.is_file() {
            Some(p.clone())
        } else {
            log_desktop_monitoring_config_error(format!(
                "--config path is not a readable file: {}",
                p.display()
            ));
            return last_good_desktop_monitoring_config();
        }
    } else {
        discover_config_path()
    };

    remember_desktop_monitoring_config_path(yaml_path.clone());

    if let Some(ref yaml_path) = yaml_path {
        match fs::read_to_string(yaml_path) {
            Ok(contents) => {
                if let Ok(cfg) = serde_yaml::from_str::<AiUsageDesktopMonitoringFile>(&contents) {
                    let config = cfg
                        .desktop_monitoring
                        .map(DesktopMonitoringConfigFile::merge_with_defaults)
                        .unwrap_or_else(DesktopMonitoringConfig::default);
                    return remember_desktop_monitoring_config(config);
                }
                log_desktop_monitoring_config_error(format!(
                    "could not parse desktop monitoring config: {}",
                    yaml_path.display()
                ));
                return last_good_desktop_monitoring_config();
            }
            Err(_) => {
                log_desktop_monitoring_config_error(format!(
                    "could not read config file: {}",
                    yaml_path.display()
                ));
                return last_good_desktop_monitoring_config();
            }
        }
    }

    remember_desktop_monitoring_config(DesktopMonitoringConfig::default())
}

pub(crate) fn reload_desktop_monitoring_config() -> DesktopMonitoringConfig {
    load_desktop_monitoring_config(last_desktop_monitoring_config_path(), || None)
}

pub(crate) fn builtin_ai_process_names() -> Vec<AiProcessConfig> {
    vec![
        ai_process(
            &["Cursor.exe", "Cursor"],
            "Cursor",
            "Anysphere",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["Claude.exe", "Claude"],
            "Claude",
            "Anthropic",
            AiProcessMatchScope::Direct,
        ),
        ai_process(
            &["claude.exe", "claude"],
            "Claude Code",
            "Anthropic",
            AiProcessMatchScope::HostedChild,
        ),
        secondary_ai_process(
            &["cowork-svc.exe", "cowork-svc"],
            "Claude Cowork",
            "Anthropic",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["Codex.exe", "codex"],
            "Codex",
            "OpenAI",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &[
                "OpenClaw.exe",
                "openclaw-desktop.exe",
                "OpenClaw.Tray.WinUI.exe",
                "OpenClaw",
                "openclaw-desktop",
            ],
            "OpenClaw",
            "OpenClaw",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["Junie.exe"],
            "Junie",
            "JetBrains",
            AiProcessMatchScope::Direct,
        ),
        ai_process(
            &["gemini.exe", "gemini-cli.exe"],
            "Gemini CLI",
            "Google",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["hermes.exe", "hermes", "hermes-agent.exe", "hermes-agent"],
            "Hermes Agent",
            "Nous Research",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["autohand.exe", "autohand-code.exe"],
            "Autohand Code CLI",
            "Autohand",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["opencode.exe"],
            "OpenCode",
            "OpenCode",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["openhands.exe"],
            "OpenHands",
            "OpenHands",
            AiProcessMatchScope::Both,
        ),
        ai_process(&["mux.exe"], "Mux", "Coder", AiProcessMatchScope::Both),
        ai_process(
            &["amp.exe"],
            "Amp",
            "Sourcegraph",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["letta.exe", "letta-code.exe"],
            "Letta",
            "Letta",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["firebender.exe"],
            "Firebender",
            "Firebender",
            AiProcessMatchScope::Direct,
        ),
        ai_process(&["goose.exe"], "Goose", "Block", AiProcessMatchScope::Both),
        ai_process(
            &["Piebald.exe", "piebald.exe"],
            "Piebald",
            "Piebald",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["factory.exe"],
            "Factory",
            "Factory",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["trae.exe", "Trae.exe"],
            "TRAE",
            "ByteDance",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["roo-code.exe", "roocode.exe"],
            "Roo Code",
            "Roo Code",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["mistral-vibe.exe", "vibe.exe"],
            "Mistral AI Vibe",
            "Mistral AI",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["command-code.exe", "commandcode.exe"],
            "Command Code",
            "Command Code",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["vtcode.exe", "vt-code.exe"],
            "VT Code",
            "VT Code",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["qodo.exe", "qodo-cli.exe"],
            "Qodo",
            "Qodo",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["kiro.exe", "Kiro.exe"],
            "Kiro",
            "Kiro",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["workshop.exe", "Workshop.exe"],
            "Workshop",
            "Workshop",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["nanobot.exe"],
            "nanobot",
            "nanobot",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["fast-agent.exe", "fastagent.exe"],
            "fast-agent",
            "fast-agent",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["tabnine.exe", "tabnine-cli.exe"],
            "Tabnine",
            "Tabnine",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["emdash.exe"],
            "Emdash",
            "Emdash",
            AiProcessMatchScope::Both,
        ),
    ]
}

pub(crate) fn builtin_host_process_names() -> Vec<String> {
    strings(&[
        "cmd.exe",
        "powershell.exe",
        "pwsh.exe",
        "WindowsTerminal.exe",
        "wt.exe",
        "conhost.exe",
        "Code.exe",
        "Cursor.exe",
        "devenv.exe",
        "idea64.exe",
        "pycharm64.exe",
        "webstorm64.exe",
        "rider64.exe",
        "wezterm-gui.exe",
        "Terminal",
        "iTerm2",
        "Ghostty",
        "WezTerm",
        "wezterm-gui",
        "Alacritty",
        "alacritty",
        "kitty",
        "Code",
        "Cursor",
        "IntelliJ IDEA",
        "PyCharm",
        "WebStorm",
        "Rider",
        "alacritty.exe",
        "mintty.exe",
        "putty.exe",
        "kitty.exe",
        "ttermpro.exe",
        "cygterm.exe",
        "tkwinterm.exe",
        "Tabby.exe",
        "Hyper.exe",
    ])
}

/// Windows foreground hosts where AttachConsole title correlation is worth trying.
///
/// This is intentionally narrower than `builtin_host_process_names`: the broader
/// host list gates process-tree matching, while this list gates the Windows-only
/// console title probe that runs near the end of detection.
#[cfg(windows)]
pub(crate) fn builtin_console_title_host_process_names() -> Vec<String> {
    strings(&[
        "WindowsTerminal.exe",
        "wt.exe",
        "cmd.exe",
        "powershell.exe",
        "pwsh.exe",
        "conhost.exe",
        "mintty.exe",
        "ttermpro.exe",
        "Tabby.exe",
        "Hyper.exe",
        "alacritty.exe",
        "wezterm-gui.exe",
    ])
}

pub(crate) fn merge_ai_process_names(
    mut builtins: Vec<AiProcessConfig>,
    overrides: Vec<AiProcessConfig>,
) -> Vec<AiProcessConfig> {
    let mut index_by_tool: HashMap<String, usize> = builtins
        .iter()
        .enumerate()
        .map(|(index, process)| (process.tool.clone(), index))
        .collect();

    for process in overrides {
        if let Some(index) = index_by_tool.get(&process.tool).copied() {
            builtins[index] = process;
        } else {
            index_by_tool.insert(process.tool.clone(), builtins.len());
            builtins.push(process);
        }
    }

    builtins
}

pub(crate) fn merge_host_process_names(
    builtins: Vec<String>,
    overrides: Vec<String>,
) -> Vec<String> {
    let mut seen: HashSet<String> = HashSet::new();
    let mut merged = Vec::new();
    for name in builtins.into_iter().chain(overrides) {
        if seen.insert(normalize_process_name(&name)) {
            merged.push(name);
        }
    }
    merged
}

fn ai_process(
    process_names: &[&str],
    tool: &str,
    provider: &str,
    match_scope: AiProcessMatchScope,
) -> AiProcessConfig {
    AiProcessConfig {
        process_names: strings(process_names),
        tool: tool.to_string(),
        provider: provider.to_string(),
        match_scope,
        approved: false,
        secondary: false,
    }
}

fn secondary_ai_process(
    process_names: &[&str],
    tool: &str,
    provider: &str,
    match_scope: AiProcessMatchScope,
) -> AiProcessConfig {
    AiProcessConfig {
        secondary: true,
        ..ai_process(process_names, tool, provider, match_scope)
    }
}

fn strings(values: &[&str]) -> Vec<String> {
    values.iter().map(|value| value.to_string()).collect()
}

fn normalize_process_name(name: &str) -> String {
    let lower = name.to_ascii_lowercase();
    lower.strip_suffix(".exe").unwrap_or(&lower).to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    static DESKTOP_CONFIG_TEST_MUTEX: Mutex<()> = Mutex::new(());

    fn parse_desktop_config(contents: &str) -> DesktopMonitoringConfig {
        serde_yaml::from_str::<AiUsageDesktopMonitoringFile>(contents)
            .expect("desktop monitoring config should parse")
            .desktop_monitoring
            .expect("desktop monitoring should be present")
            .merge_with_defaults()
    }

    #[test]
    fn desktop_monitoring_debug_defaults_to_off() {
        let cfg = DesktopMonitoringConfig::default();

        assert_eq!(cfg.debug, 0);
    }

    #[test]
    fn desktop_monitoring_defaults_are_loaded_from_builtins() {
        let cfg = DesktopMonitoringConfig::default();

        assert!(
            cfg.ai_process_names
                .iter()
                .any(|process| process.tool == "Cursor")
        );
        assert!(
            cfg.host_process_names
                .iter()
                .any(|process| process == "Code")
        );
        assert_eq!(cfg.process_activity_window_seconds, 600);
    }

    #[test]
    fn desktop_monitoring_partial_yaml_preserves_builtin_process_lists() {
        let cfg = parse_desktop_config(
            r#"
desktop_monitoring:
  debug: 2
  process_activity_window_seconds: 300
"#,
        );

        assert_eq!(cfg.debug, 2);
        assert_eq!(cfg.process_activity_window_seconds, 300);
        assert!(
            cfg.ai_process_names
                .iter()
                .any(|process| process.tool == "Cursor")
        );
        assert!(
            cfg.host_process_names
                .iter()
                .any(|process| process == "Terminal")
        );
    }

    #[test]
    fn desktop_monitoring_yaml_ai_tools_override_builtin_tools_by_tool_name() {
        let cfg = parse_desktop_config(
            r#"
desktop_monitoring:
  ai_process_names:
    - process_names: ["claude-beta"]
      tool: "Claude Code"
      provider: "Anthropic Labs"
      match_scope: "direct"
      approved: true
"#,
        );
        let claude_code: Vec<_> = cfg
            .ai_process_names
            .iter()
            .filter(|process| process.tool == "Claude Code")
            .collect();

        assert_eq!(claude_code.len(), 1);
        assert_eq!(claude_code[0].process_names, vec!["claude-beta"]);
        assert_eq!(claude_code[0].provider, "Anthropic Labs");
        assert_eq!(claude_code[0].match_scope, AiProcessMatchScope::Direct);
        assert!(claude_code[0].approved);
    }

    #[test]
    fn desktop_monitoring_yaml_ai_tools_add_new_tools() {
        let cfg = parse_desktop_config(
            r#"
desktop_monitoring:
  ai_process_names:
    - process_names: ["new-tool"]
      tool: "New Tool"
      provider: "Example"
      match_scope: "hosted_child"
"#,
        );

        assert!(
            cfg.ai_process_names
                .iter()
                .any(|process| process.tool == "Cursor")
        );
        assert!(
            cfg.ai_process_names
                .iter()
                .any(|process| process.tool == "New Tool")
        );
    }

    #[test]
    fn desktop_monitoring_yaml_host_names_are_merged_and_deduplicated() {
        let cfg = parse_desktop_config(
            r#"
desktop_monitoring:
  host_process_names:
    - "Code.exe"
    - "CustomTerminal"
"#,
        );

        assert_eq!(
            cfg.host_process_names
                .iter()
                .filter(|process| process.as_str() == "Code.exe")
                .count(),
            1
        );
        assert!(
            cfg.host_process_names
                .iter()
                .any(|process| process == "CustomTerminal")
        );
    }

    #[test]
    fn invalid_desktop_monitoring_yaml_uses_builtin_config_on_first_load() {
        let _guard = DESKTOP_CONFIG_TEST_MUTEX.lock().expect("test mutex");
        reset_desktop_monitoring_config_cache();
        let path = std::env::temp_dir().join(format!(
            "ai_usage_native_host_invalid_first_load_{}.yaml",
            std::process::id()
        ));
        std::fs::write(&path, "desktop_monitoring: [").expect("invalid YAML should be written");

        let cfg = load_desktop_monitoring_config(Some(path.clone()), || None);
        let _ = std::fs::remove_file(path);

        assert!(cfg.enabled);
        assert_eq!(cfg.debug, 0);
        assert_eq!(cfg.poll_interval_seconds, 60);
        assert!(
            cfg.ai_process_names
                .iter()
                .any(|process| process.tool == "Cursor")
        );
        assert!(
            cfg.host_process_names
                .iter()
                .any(|process| process == "Code")
        );
    }

    #[test]
    fn invalid_desktop_monitoring_yaml_returns_last_good_config() {
        let _guard = DESKTOP_CONFIG_TEST_MUTEX.lock().expect("test mutex");
        reset_desktop_monitoring_config_cache();
        let path = std::env::temp_dir().join(format!(
            "ai_usage_native_host_last_good_{}.yaml",
            std::process::id()
        ));
        std::fs::write(
            &path,
            r#"
desktop_monitoring:
  debug: 2
  poll_interval_seconds: 7
"#,
        )
        .expect("valid YAML should be written");

        let first_cfg = load_desktop_monitoring_config(Some(path.clone()), || None);
        assert_eq!(first_cfg.debug, 2);
        assert_eq!(first_cfg.poll_interval_seconds, 7);

        std::fs::write(&path, "desktop_monitoring: [").expect("invalid YAML should be written");
        let fallback_cfg = load_desktop_monitoring_config(Some(path.clone()), || None);
        let _ = std::fs::remove_file(path);

        assert!(fallback_cfg.enabled);
        assert_eq!(fallback_cfg.debug, 2);
        assert_eq!(fallback_cfg.poll_interval_seconds, 7);
    }

    #[test]
    fn desktop_monitoring_debug_level_can_be_configured_from_yaml() {
        let cfg = parse_desktop_config(
            r#"
desktop_monitoring:
  debug: 2
"#,
        );

        assert_eq!(cfg.debug, 2);
    }

    #[test]
    fn desktop_monitoring_process_activity_window_can_be_configured_from_yaml() {
        let cfg = parse_desktop_config(
            r#"
desktop_monitoring:
  process_activity_window_seconds: 300
"#,
        );

        assert_eq!(cfg.process_activity_window_seconds, 300);
    }

    #[test]
    fn desktop_monitoring_accepts_legacy_terminal_activity_window_key() {
        let cfg = parse_desktop_config(
            r#"
desktop_monitoring:
  terminal_activity_window_seconds: 300
"#,
        );

        assert_eq!(cfg.process_activity_window_seconds, 300);
    }
}
