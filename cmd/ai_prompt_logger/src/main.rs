mod datadog;
mod desktop;
mod protocol;

use std::env;
use std::path::PathBuf;

use anyhow::Result;
use serde::{Deserialize, Serialize};

use crate::datadog::{AiUsageEvent, DatadogClient, resolve_hostname};
use crate::protocol::{read_message, write_message};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum RunMode {
    NativeHost,
    DesktopMonitor,
}

#[derive(Debug)]
struct CliArgs {
    config_path: Option<PathBuf>,
    mode: RunMode,
}

#[derive(Debug, Deserialize)]
#[serde(tag = "type")]
enum Request {
    #[serde(rename = "HEALTH_CHECK")]
    HealthCheck,
    #[serde(rename = "SEND_USAGE_EVENT")]
    SendUsageEvent {
        tool: String,
        #[serde(default)]
        provider: Option<String>,
        user_id: String,
        #[serde(default)]
        approved: bool,
    },
}

#[derive(Debug, Serialize)]
#[serde(tag = "type")]
enum Response {
    #[serde(rename = "HEALTH_RESULT")]
    HealthResult { status: String },
    #[serde(rename = "SEND_USAGE_EVENT_RESULT")]
    SendUsageEventResult { success: bool },
    #[serde(rename = "ERROR")]
    Error { error: String },
}

fn is_recoverable_read_error(error: &anyhow::Error) -> bool {
    let flattened = format!("{:#}", error);
    // Oversized declared lengths are not recoverable: we do not drain the body, so stdin
    // framing is desynchronized until the process exits.
    flattened.contains("Failed to parse JSON message")
}

/// Parse `--desktop-monitor`, `--config=PATH`, `--config PATH`, or `-c PATH`.
/// Unknown arguments are ignored because Chrome may pass browser-owned arguments
/// such as `--parent-window=...` on Windows.
fn parse_args() -> CliArgs {
    let args: Vec<String> = env::args().collect();
    let mut i = 1usize;
    let mut config_path: Option<PathBuf> = None;
    let mut mode = RunMode::NativeHost;
    while i < args.len() {
        let arg = args[i].as_str();
        if arg == "-h" || arg == "--help" {
            std::process::exit(0);
        }
        if arg == "--desktop-monitor" {
            mode = RunMode::DesktopMonitor;
            i += 1;
            continue;
        }
        if let Some(rest) = arg.strip_prefix("--config=") {
            if rest.is_empty() {
                std::process::exit(2);
            }
            config_path = Some(PathBuf::from(rest));
            i += 1;
            continue;
        }
        if arg == "--config" || arg == "-c" {
            if i + 1 >= args.len() {
                std::process::exit(2);
            }
            config_path = Some(PathBuf::from(&args[i + 1]));
            i += 2;
            continue;
        }
        i += 1;
    }
    CliArgs { config_path, mode }
}

fn handle_message(dd_client: &DatadogClient, request: Request) -> Response {
    match request {
        Request::HealthCheck => Response::HealthResult {
            status: "ok".to_string(),
        },
        Request::SendUsageEvent {
            tool,
            provider,
            user_id,
            approved,
        } => {
            let resolved_host = resolve_hostname();
            let mut event = AiUsageEvent::new("observed", tool, user_id, resolved_host, approved);
            let prov = provider
                .as_deref()
                .filter(|s| !s.is_empty())
                .unwrap_or("unknown")
                .to_string();
            event.provider = Some(prov);
            let success = dd_client.send_event(&event);
            Response::SendUsageEventResult { success }
        }
    }
}

fn run_native_host(dd_client: &DatadogClient) -> Result<()> {
    loop {
        match read_message() {
            Ok(Some(value)) => match serde_json::from_value::<Request>(value) {
                Ok(request) => {
                    let response = handle_message(dd_client, request);
                    if write_message(&response).is_err() {
                        break;
                    }
                }
                Err(e) => {
                    let response = Response::Error {
                        error: format!("Invalid request: {}", e),
                    };
                    if write_message(&response).is_err() {
                        break;
                    }
                }
            },
            Ok(None) => {
                break;
            }
            Err(e) => {
                if is_recoverable_read_error(&e) {
                    continue;
                }
                break;
            }
        }
    }

    Ok(())
}

fn detach_desktop_monitor_console() {
    #[cfg(windows)]
    unsafe {
        // The binary stays a console-subsystem executable for Chrome native messaging.
        // Desktop monitor mode is launched as a user-session background task, so detach
        // from the scheduler-created console after mode selection.
        windows_sys::Win32::System::Console::FreeConsole();
    }
}

fn main() -> Result<()> {
    let cli_args = parse_args();
    if cli_args.mode == RunMode::DesktopMonitor {
        detach_desktop_monitor_console();
    }

    let dd_client = DatadogClient::load(cli_args.config_path.clone());

    match cli_args.mode {
        RunMode::NativeHost => run_native_host(&dd_client),
        RunMode::DesktopMonitor => {
            let config = DatadogClient::load_desktop_monitoring_config(cli_args.config_path);
            desktop::run(&dd_client, config)
        }
    }
}
