//! Output formatting utilities

use crate::process_manager::ProcessState;
use chrono::{Local, TimeZone};
use colored::*;

/// Format a Unix timestamp to human-readable date/time
pub fn format_timestamp(ts: i64) -> String {
    if ts == 0 {
        return "-".to_string();
    }
    match Local.timestamp_opt(ts, 0) {
        chrono::LocalResult::Single(dt) => dt.format("%Y-%m-%d %H:%M:%S").to_string(),
        _ => "invalid".to_string(),
    }
}

/// Format process state with appropriate color
pub fn format_state(state: ProcessState) -> ColoredString {
    let state_str = format!("{:?}", state).to_lowercase();
    match state {
        ProcessState::Running => state_str.green(),
        ProcessState::Crashed => state_str.red(),
        ProcessState::Exited => state_str.yellow(),
        ProcessState::Stopping => state_str.yellow(),
        ProcessState::Created => state_str.cyan(),
        _ => state_str.normal(),
    }
}
