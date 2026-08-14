use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};

use chrono::Utc;

const LOG_FILE_NAME: &str = "ai-usage-desktop-monitor.log";
const MAX_LOG_BYTES: u64 = 10 * 1024 * 1024;

#[derive(Debug, Clone)]
pub struct DesktopLogger {
    path: Option<PathBuf>,
    debug_level: u8,
    pid: u32,
    user: String,
}

impl DesktopLogger {
    pub fn new(debug_level: u8) -> Self {
        Self {
            path: (debug_level > 0).then(resolve_log_path).flatten(),
            debug_level,
            pid: std::process::id(),
            user: resolve_user(),
        }
    }

    pub fn info(&self, message: impl AsRef<str>) {
        self.write("INFO", message.as_ref());
    }

    pub fn warn(&self, message: impl AsRef<str>) {
        self.write("WARN", message.as_ref());
    }

    pub fn info_at(&self, debug_level: u8, message: impl AsRef<str>) {
        if self.debug_level >= debug_level {
            self.info(message);
        }
    }

    fn write(&self, level: &str, message: &str) {
        let Some(path) = &self.path else {
            return;
        };

        rotate_if_needed(path);

        let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) else {
            return;
        };

        let _ = writeln!(
            file,
            "{} pid={} user=\"{}\" level={} msg=\"{}\"",
            Utc::now().to_rfc3339_opts(chrono::SecondsFormat::Millis, true),
            self.pid,
            escape_log_value(&self.user),
            level,
            escape_log_value(message)
        );
    }
}

fn resolve_log_path() -> Option<PathBuf> {
    candidate_log_dirs()
        .into_iter()
        .map(|dir| dir.join(LOG_FILE_NAME))
        .find(|path| ensure_log_file(path))
}

fn candidate_log_dirs() -> Vec<PathBuf> {
    let mut dirs = Vec::new();
    if let Some(dd_log_dir) = non_empty_env("DD_LOG_DIR") {
        dirs.push(PathBuf::from(dd_log_dir));
    }

    #[cfg(windows)]
    {
        if let Some(program_data) = non_empty_env("ProgramData") {
            dirs.push(PathBuf::from(program_data).join("Datadog").join("logs"));
        }
        if let Some(local_app_data) = non_empty_env("LOCALAPPDATA") {
            dirs.push(PathBuf::from(local_app_data).join("Datadog").join("logs"));
        }
    }

    #[cfg(target_os = "macos")]
    {
        dirs.push(PathBuf::from("/opt/datadog-agent/logs"));
        if let Some(home) = non_empty_env("HOME") {
            dirs.push(
                PathBuf::from(home)
                    .join("Library")
                    .join("Logs")
                    .join("Datadog"),
            );
        }
    }

    dirs
}

fn ensure_log_file(path: &Path) -> bool {
    let Some(parent) = path.parent() else {
        return false;
    };
    fs::create_dir_all(parent).is_ok()
        && OpenOptions::new()
            .create(true)
            .append(true)
            .open(path)
            .is_ok()
}

fn rotate_if_needed(path: &Path) {
    let Ok(metadata) = fs::metadata(path) else {
        return;
    };
    if metadata.len() < MAX_LOG_BYTES {
        return;
    }

    let backup = PathBuf::from(format!("{}.1", path.display()));
    let _ = fs::remove_file(&backup);
    let _ = fs::rename(path, backup);
}

fn resolve_user() -> String {
    match (non_empty_env("USERDOMAIN"), non_empty_env("USERNAME")) {
        (Some(domain), Some(user)) => format!("{domain}\\{user}"),
        (_, Some(user)) => user,
        _ => non_empty_env("USER").unwrap_or_else(|| "unknown".to_string()),
    }
}

fn non_empty_env(name: &str) -> Option<String> {
    std::env::var(name).ok().filter(|value| !value.is_empty())
}

fn escape_log_value(value: &str) -> String {
    value.replace('\\', "\\\\").replace('"', "\\\"")
}
