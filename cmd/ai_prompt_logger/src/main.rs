mod datadog;
mod protocol;

use std::env;
use std::path::PathBuf;
use std::process;

use anyhow::Result;
use serde::{Deserialize, Serialize};

use crate::datadog::{AiUsageEvent, DatadogClient, resolve_hostname};
use crate::protocol::{read_message, write_message};

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
        hostname: Option<String>,
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
    flattened.contains("Message too large") || flattened.contains("Failed to parse JSON message")
}

/// Parse `--config=PATH`, `--config PATH`, or `-c PATH` (same idea as `agent run -c` /
/// `system-probe --config`). Unknown arguments are rejected.
fn config_path_from_args() -> Option<PathBuf> {
    let args: Vec<String> = env::args().collect();
    let mut i = 1usize;
    let mut found: Option<PathBuf> = None;
    while i < args.len() {
        let arg = args[i].as_str();
        if arg == "-h" || arg == "--help" {
            print_help();
            process::exit(0);
        }
        if let Some(rest) = arg.strip_prefix("--config=") {
            if rest.is_empty() {
                eprintln!("error: --config= requires a path");
                process::exit(2);
            }
            found = Some(PathBuf::from(rest));
            i += 1;
            continue;
        }
        if arg == "--config" || arg == "-c" {
            if i + 1 >= args.len() {
                eprintln!("error: {} requires a path argument", arg);
                process::exit(2);
            }
            found = Some(PathBuf::from(&args[i + 1]));
            i += 2;
            continue;
        }
        eprintln!("error: unexpected argument: {arg}");
        print_help();
        process::exit(2);
    }
    found
}

fn print_help() {
    eprintln!(
        "\
Usage: ai-prompt-logger-native-host [--config=PATH | --config PATH | -c PATH]

  --config PATH, --config=PATH, -c PATH   YAML file (same role as agent -c / system-probe --config).
                                           When omitted, searches for ai_usage_native_host.yaml
                                           under the Agent install prefix (see packaged docs).

Examples:
  ai-prompt-logger-native-host --config=/opt/datadog-agent/etc/ai_usage_native_host.yaml
  ai-prompt-logger-native-host -c ./ai_usage_native_host.yaml
"
    );
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
            hostname,
            approved,
        } => {
            let resolved_host = resolve_hostname(hostname.as_deref());
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

fn main() -> Result<()> {
    eprintln!("[native-host-rs] Starting...");

    let dd_client = DatadogClient::load(config_path_from_args());

    loop {
        match read_message() {
            Ok(Some(value)) => match serde_json::from_value::<Request>(value) {
                Ok(request) => {
                    let response = handle_message(&dd_client, request);
                    if let Err(e) = write_message(&response) {
                        eprintln!("[native-host-rs] Failed to write response: {}", e);
                        break;
                    }
                }
                Err(e) => {
                    let response = Response::Error {
                        error: format!("Invalid request: {}", e),
                    };
                    if let Err(e) = write_message(&response) {
                        eprintln!("[native-host-rs] Failed to write error response: {}", e);
                        break;
                    }
                }
            },
            Ok(None) => {
                eprintln!("[native-host-rs] stdin closed, exiting");
                break;
            }
            Err(e) => {
                if is_recoverable_read_error(&e) {
                    eprintln!(
                        "[native-host-rs] Recoverable read error, skipping message: {}",
                        e
                    );
                    continue;
                }
                eprintln!("[native-host-rs] Fatal read error, exiting: {}", e);
                break;
            }
        }
    }

    Ok(())
}
