// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use log::{Log, Metadata, Record};
use std::fs::{File, OpenOptions};
use std::io::Write;
use std::path::PathBuf;
use std::sync::Mutex;
use time::OffsetDateTime;

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

        let timestamp = OffsetDateTime::now_utc()
            .format(time::macros::format_description!(
                "[year]-[month]-[day] [hour]:[minute]:[second] UTC"
            ))
            .unwrap();
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
    fn test_last_module_segment() {
        assert_eq!(last_module_segment("dd_discovery::injector"), "injector");
        assert_eq!(last_module_segment("main"), "main");
        assert_eq!(last_module_segment("a::b::c"), "c");
        assert_eq!(last_module_segment(""), "");
    }
}
