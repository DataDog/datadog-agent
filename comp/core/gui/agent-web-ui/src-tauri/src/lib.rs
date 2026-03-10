use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;
#[cfg(debug_assertions)]
use tauri::Manager;

// ---------------------------------------------------------------------------
// Platform paths
// ---------------------------------------------------------------------------

#[cfg(target_os = "macos")]
mod paths {
    pub const CONF_DIR: &str = "/opt/datadog-agent/etc";
    pub const LOG_FILE: &str = "/opt/datadog-agent/logs/agent.log";
    pub const CONFD_DIR: &str = "/opt/datadog-agent/etc/conf.d";
    pub const CHECKS_D_DIR: &str = "/opt/datadog-agent/etc/checks.d";
    pub const AGENT_BIN: &str = "/opt/datadog-agent/bin/agent/agent";
    pub const LAUNCHCTL_SERVICE: &str = "com.datadoghq.agent";
}

#[cfg(target_os = "linux")]
mod paths {
    pub const CONF_DIR: &str = "/etc/datadog-agent";
    pub const LOG_FILE: &str = "/var/log/datadog/agent.log";
    pub const CONFD_DIR: &str = "/etc/datadog-agent/conf.d";
    pub const CHECKS_D_DIR: &str = "/etc/datadog-agent/checks.d";
    pub const AGENT_BIN: &str = "/opt/datadog-agent/bin/agent/agent";
}

#[cfg(target_os = "windows")]
mod paths {
    pub const CONF_DIR: &str = "C:\\ProgramData\\Datadog";
    pub const LOG_FILE: &str = "C:\\ProgramData\\Datadog\\logs\\agent.log";
    pub const CONFD_DIR: &str = "C:\\ProgramData\\Datadog\\conf.d";
    pub const CHECKS_D_DIR: &str = "C:\\ProgramData\\Datadog\\checks.d";
    pub const AGENT_BIN: &str = "C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe";
}

fn config_path() -> PathBuf {
    Path::new(paths::CONF_DIR).join("datadog.yaml")
}

fn run_agent(args: &[&str]) -> Result<String, String> {
    let output = Command::new(paths::AGENT_BIN)
        .args(args)
        .output()
        .map_err(|e| format!("Failed to run agent: {}", e))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!("Agent command failed: {}", stderr));
    }
    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

#[derive(Serialize, Deserialize, Debug)]
pub struct AgentVersion {
    pub major: String,
    pub minor: String,
    pub patch: String,
    pub commit: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AgentInfo {
    pub version: String,
    pub hostname: String,
    pub running: bool,
}

// ---------------------------------------------------------------------------
// Agent commands
// ---------------------------------------------------------------------------

#[tauri::command]
fn agent_ping() -> Result<bool, String> {
    #[cfg(target_os = "macos")]
    {
        let output = Command::new("/bin/launchctl")
            .args(["list", paths::LAUNCHCTL_SERVICE])
            .output()
            .map_err(|e| e.to_string())?;
        let stdout = String::from_utf8_lossy(&output.stdout);
        Ok(output.status.success() && stdout.contains("\"PID\""))
    }
    #[cfg(not(target_os = "macos"))]
    {
        let output = Command::new(paths::AGENT_BIN)
            .args(["status"])
            .output()
            .map_err(|e| e.to_string())?;
        Ok(output.status.success())
    }
}

#[tauri::command]
fn agent_status(status_type: String) -> Result<String, String> {
    match status_type.as_str() {
        "general" => run_agent(&["status"]),
        "collector" => run_agent(&["status", "collector"]),
        _ => Err(format!("Unknown status type: {}", status_type)),
    }
}

#[tauri::command]
fn agent_version() -> Result<String, String> {
    run_agent(&["version"])
}

#[tauri::command]
fn agent_hostname() -> Result<String, String> {
    run_agent(&["hostname"])
}

#[tauri::command]
fn agent_log(flip: bool) -> Result<Vec<String>, String> {
    let contents =
        fs::read_to_string(paths::LOG_FILE).map_err(|e| format!("Error reading log: {}", e))?;

    let mut lines: Vec<String> = contents.lines().map(|l| l.to_string()).collect();
    if flip {
        lines.reverse();
    }
    Ok(lines)
}

#[tauri::command]
fn agent_flare(email: String, case_id: String) -> Result<String, String> {
    run_agent(&["flare", &case_id, "--email", &email, "--send"])
}

#[tauri::command]
fn agent_restart() -> Result<String, String> {
    #[cfg(target_os = "macos")]
    {
        Command::new("/bin/launchctl")
            .args(["stop", paths::LAUNCHCTL_SERVICE])
            .output()
            .map_err(|e| e.to_string())?;
        std::thread::sleep(std::time::Duration::from_secs(2));
        Command::new("/bin/launchctl")
            .args(["start", paths::LAUNCHCTL_SERVICE])
            .output()
            .map_err(|e| e.to_string())?;
        Ok("Success".to_string())
    }
    #[cfg(target_os = "linux")]
    {
        Command::new("sudo")
            .args(["systemctl", "restart", "datadog-agent"])
            .output()
            .map_err(|e| e.to_string())?;
        Ok("Success".to_string())
    }
    #[cfg(target_os = "windows")]
    {
        Command::new("powershell")
            .args(["-Command", "Restart-Service datadogagent"])
            .output()
            .map_err(|e| e.to_string())?;
        Ok("Success".to_string())
    }
}

#[tauri::command]
fn agent_get_config() -> Result<String, String> {
    fs::read_to_string(config_path()).map_err(|e| format!("Error reading config: {}", e))
}

#[tauri::command]
fn agent_set_config(config: String) -> Result<String, String> {
    // Validate YAML before writing
    serde_yaml::from_str::<serde_yaml::Value>(&config)
        .map_err(|e| format!("Invalid YAML: {}", e))?;

    fs::write(config_path(), &config).map_err(|e| format!("Error writing config: {}", e))?;
    Ok("Success".to_string())
}

// ---------------------------------------------------------------------------
// Check commands
// ---------------------------------------------------------------------------

#[tauri::command]
fn checks_running() -> Result<String, String> {
    run_agent(&["check", "--list"])
}

fn has_config_extension(name: &str) -> bool {
    let lower = name.to_lowercase();
    lower.ends_with(".yaml")
        || lower.ends_with(".yml")
        || lower.ends_with(".yaml.default")
        || lower.ends_with(".yml.default")
        || lower.ends_with(".yaml.disabled")
        || lower.ends_with(".yml.disabled")
        || lower.ends_with(".yaml.example")
        || lower.ends_with(".yml.example")
}

fn list_configs_in_dir(dir: &Path) -> Vec<String> {
    let mut results = Vec::new();
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return results,
    };

    for entry in entries.flatten() {
        let name = entry.file_name().to_string_lossy().to_string();
        let path = entry.path();

        if path.is_dir() && name.ends_with(".d") {
            if let Ok(sub_entries) = fs::read_dir(&path) {
                for sub in sub_entries.flatten() {
                    let sub_name = sub.file_name().to_string_lossy().to_string();
                    if sub.path().is_file() && has_config_extension(&sub_name) {
                        results.push(format!("{}/{}", name, sub_name));
                    }
                }
            }
        } else if path.is_file() && has_config_extension(&name) {
            results.push(name);
        }
    }
    results.sort();
    results
}

#[tauri::command]
fn checks_list_configs() -> Result<Vec<String>, String> {
    let mut all = Vec::new();
    for dir in [paths::CONFD_DIR] {
        all.extend(list_configs_in_dir(Path::new(dir)));
    }
    Ok(all)
}

#[tauri::command]
fn checks_list_checks() -> Result<Vec<String>, String> {
    let mut integrations = Vec::new();
    for dir_path in [paths::CHECKS_D_DIR] {
        if let Ok(entries) = fs::read_dir(dir_path) {
            for entry in entries.flatten() {
                let name = entry.file_name().to_string_lossy().to_string();
                if entry.path().is_file() && name.ends_with(".py") {
                    integrations.push(name);
                }
            }
        }
    }
    integrations.sort();
    Ok(integrations)
}

#[tauri::command]
fn checks_get_config(file_name: String) -> Result<String, String> {
    if file_name.contains("..") {
        return Err("Invalid path".to_string());
    }

    let path = Path::new(paths::CONFD_DIR).join(&file_name);
    fs::read_to_string(&path).map_err(|e| format!("Error reading {}: {}", file_name, e))
}

#[tauri::command]
fn checks_set_config(file_name: String, config: String) -> Result<String, String> {
    if file_name.contains("..") {
        return Err("Invalid path".to_string());
    }

    // Validate YAML
    serde_yaml::from_str::<serde_yaml::Value>(&config)
        .map_err(|e| format!("Invalid YAML: {}", e))?;

    let path = Path::new(paths::CONFD_DIR).join(&file_name);
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| format!("Error creating directory: {}", e))?;
    }
    fs::write(&path, &config).map_err(|e| format!("Error writing config: {}", e))?;
    Ok("Success".to_string())
}

#[tauri::command]
fn checks_disable(file_name: String) -> Result<String, String> {
    if file_name.contains("..") {
        return Err("Invalid path".to_string());
    }

    let path = Path::new(paths::CONFD_DIR).join(&file_name);
    let disabled_path = path.with_extension(format!(
        "{}.disabled",
        path.extension().unwrap_or_default().to_string_lossy()
    ));
    fs::rename(&path, &disabled_path).map_err(|e| format!("Error disabling check: {}", e))?;
    Ok("Success".to_string())
}

// ---------------------------------------------------------------------------
// App entry
// ---------------------------------------------------------------------------

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .setup(|app| {
            #[cfg(debug_assertions)]
            {
                if let Some(w) = app.get_webview_window("main") {
                    w.open_devtools();
                }
            }
            let _ = app;
            Ok(())
        })
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            agent_ping,
            agent_status,
            agent_version,
            agent_hostname,
            agent_log,
            agent_flare,
            agent_restart,
            agent_get_config,
            agent_set_config,
            checks_running,
            checks_list_checks,
            checks_list_configs,
            checks_get_config,
            checks_set_config,
            checks_disable,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
