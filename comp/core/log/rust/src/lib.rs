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

    fn make_logger(file: File, level: log::Level) -> DdAgentLogger {
        DdAgentLogger {
            logger_name: "TEST",
            level,
            file: Mutex::new(Some(file)),
        }
    }

    macro_rules! log_record {
        ($level:expr, $msg:literal) => {
            Record::builder()
                .args(format_args!($msg))
                .level($level)
                .file(Some("src/main.rs"))
                .line(Some(42))
                .module_path(Some("my_crate::my_module"))
                .build()
        };
    }

    // The log output format must match the agent log grok parsing rules in
    // integrations-core so that log management can parse agent logs:
    // https://github.com/DataDog/integrations-core/blob/2a5ad5eb40a777ff1cc80db054edf57c3cd7178b/datadog_cluster_agent/assets/logs/agent.yaml#L26-L41
    //
    // Expected format:
    //   YYYY-MM-DD HH:MM:SS UTC | <logger_name> | <LEVEL> | (<file>:<line> in <module>) | <message>
    fn log_line_regex() -> regex::Regex {
        regex::Regex::new(
            r"^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} UTC \| (\S+) \| (\w+) \| \(([^:]+):(\d+) in (\S+)\) \| (.+)$",
        )
        .unwrap()
    }

    #[test]
    fn test_log_writes_to_file() {
        let tmp = tempfile::NamedTempFile::new().unwrap();
        let file = tmp.reopen().unwrap();
        let logger = make_logger(file, log::Level::Info);

        logger.log(&log_record!(log::Level::Info, "hello world"));

        let content = std::fs::read_to_string(tmp.path()).unwrap();
        let line = content.trim_end();
        let re = log_line_regex();
        let caps = re
            .captures(line)
            .unwrap_or_else(|| panic!("log line did not match expected format: {line}"));
        assert_eq!(&caps[1], "TEST");
        assert_eq!(&caps[2], "INFO");
        assert_eq!(&caps[3], "src/main.rs");
        assert_eq!(&caps[4], "42");
        assert_eq!(&caps[5], "my_module");
        assert_eq!(&caps[6], "hello world");
    }

    #[test]
    fn test_log_format_fields() {
        let tmp = tempfile::NamedTempFile::new().unwrap();
        let file = tmp.reopen().unwrap();
        let logger = make_logger(file, log::Level::Warn);

        logger.log(&log_record!(log::Level::Warn, "something bad"));

        let content = std::fs::read_to_string(tmp.path()).unwrap();
        let line = content.trim_end();
        let re = log_line_regex();
        let caps = re
            .captures(line)
            .unwrap_or_else(|| panic!("log line did not match expected format: {line}"));
        assert_eq!(&caps[1], "TEST");
        assert_eq!(&caps[2], "WARN");
        assert_eq!(&caps[3], "src/main.rs");
        assert_eq!(&caps[4], "42");
        assert_eq!(&caps[5], "my_module");
        assert_eq!(&caps[6], "something bad");
    }

    #[test]
    fn test_log_level_filtering() {
        let tmp = tempfile::NamedTempFile::new().unwrap();
        let file = tmp.reopen().unwrap();
        let logger = make_logger(file, log::Level::Warn);

        // INFO is below WARN, should be filtered out
        logger.log(&log_record!(log::Level::Info, "should not appear"));

        let content = std::fs::read_to_string(tmp.path()).unwrap();
        assert!(content.is_empty(), "expected no output, got: {content}");
    }

    #[test]
    fn test_log_creates_parent_dirs() {
        let tmp_dir = tempfile::tempdir().unwrap();
        let log_path = tmp_dir
            .path()
            .join("subdir")
            .join("nested")
            .join("agent.log");

        assert!(!log_path.parent().unwrap().exists());

        let _ = init(LogConfig {
            logger_name: "TEST",
            level: log::Level::Info,
            log_file: Some(log_path.clone()),
        });
        // init() may fail due to set_boxed_logger being called once per process,
        // but the directory creation happens before that call.
        assert!(
            log_path.parent().unwrap().exists(),
            "parent directories should have been created"
        );
    }
}
