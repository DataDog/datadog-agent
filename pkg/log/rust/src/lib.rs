// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use log::{Log, Metadata, Record};
use std::fs::{File, OpenOptions};
use std::io::Write;
use std::path::PathBuf;
use std::sync::Mutex;
use std::time::SystemTime;

pub struct LogConfig {
    pub logger_name: &'static str,
    pub level: log::Level,
    pub log_file: Option<PathBuf>,
}

struct DdAgentLogger {
    logger_name: &'static str,
    level: log::Level,
    file: Mutex<Option<File>>,
}

/// Formats a `SystemTime` as `YYYY-MM-DD HH:MM:SS UTC` using pure Rust (no chrono/libc).
fn format_utc_time(time: SystemTime) -> String {
    let duration = time
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default();
    let secs = duration.as_secs();

    // Days and time-of-day
    let days = secs / 86400;
    let remaining = secs % 86400;
    let hours = remaining / 3600;
    let minutes = (remaining % 3600) / 60;
    let seconds = remaining % 60;

    // Convert days since epoch to year/month/day
    // Using the algorithm from http://howardhinnant.github.io/date_algorithms.html
    let z = days + 719_468;
    let era = z / 146_097;
    let doe = z - era * 146_097;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146_096) / 365;
    let y = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = if m <= 2 { y + 1 } else { y };

    format!("{y:04}-{m:02}-{d:02} {hours:02}:{minutes:02}:{seconds:02} UTC")
}

/// Returns the last segment of a module path (e.g. `injector` from `dd_discovery::injector`).
fn last_module_segment(module_path: &str) -> &str {
    module_path.rsplit("::").next().unwrap_or(module_path)
}

impl Log for DdAgentLogger {
    fn enabled(&self, metadata: &Metadata<'_>) -> bool {
        metadata.level() <= self.level
    }

    fn log(&self, record: &Record<'_>) {
        if !self.enabled(record.metadata()) {
            return;
        }

        let timestamp = format_utc_time(SystemTime::now());
        let level = record.level();
        let file_path = record.file().unwrap_or("<unknown>");
        let line = record.line().unwrap_or(0);
        let module = record
            .module_path()
            .map(last_module_segment)
            .unwrap_or("<unknown>");

        let msg = format!(
            "{timestamp} | {} | {level} | ({file_path}:{line} in {module}) | {}\n",
            self.logger_name,
            record.args(),
        );

        let _ = std::io::stdout().write_all(msg.as_bytes());

        if let Ok(mut guard) = self.file.lock()
            && let Some(ref mut f) = *guard
        {
            let _ = f.write_all(msg.as_bytes());
        }
    }

    fn flush(&self) {
        let _ = std::io::stdout().flush();
        if let Ok(mut guard) = self.file.lock()
            && let Some(ref mut f) = *guard
        {
            let _ = f.flush();
        }
    }
}

pub fn init(config: LogConfig) -> Result<(), log::SetLoggerError> {
    let file = config.log_file.and_then(|path| {
        // Create parent directories if needed
        if let Some(parent) = path.parent()
            && !parent.exists()
        {
            let _ = std::fs::create_dir_all(parent);
        }
        match OpenOptions::new().append(true).create(true).open(&path) {
            Ok(f) => Some(f),
            Err(e) => {
                let _ = std::io::stderr().write_all(
                    format!(
                        "WARNING: could not open log file {}: {e}, logging to stdout only\n",
                        path.display()
                    )
                    .as_bytes(),
                );
                None
            }
        }
    });

    let logger = Box::new(DdAgentLogger {
        logger_name: config.logger_name,
        level: config.level,
        file: Mutex::new(file),
    });

    log::set_boxed_logger(logger)?;
    log::set_max_level(config.level.to_level_filter());
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_format_utc_time() {
        // Unix epoch
        let epoch = SystemTime::UNIX_EPOCH;
        assert_eq!(format_utc_time(epoch), "1970-01-01 00:00:00 UTC");
    }

    #[test]
    fn test_format_utc_time_known_date() {
        use std::time::Duration;
        // 2026-03-05 09:54:22 UTC = 1772704462 seconds since epoch
        let time = SystemTime::UNIX_EPOCH + Duration::from_secs(1_772_704_462);
        assert_eq!(format_utc_time(time), "2026-03-05 09:54:22 UTC");
    }

    #[test]
    fn test_last_module_segment() {
        assert_eq!(last_module_segment("dd_discovery::injector"), "injector");
        assert_eq!(last_module_segment("main"), "main");
        assert_eq!(last_module_segment("a::b::c"), "c");
        assert_eq!(last_module_segment(""), "");
    }
}
