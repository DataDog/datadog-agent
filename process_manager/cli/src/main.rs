mod commands;
mod formatters;
mod options;

use std::env;

mod process_manager {
    tonic::include_proto!("process_manager");
}
use process_manager::process_manager_client::ProcessManagerClient;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = env::args().collect();

    if args.len() < 2 {
        print_usage();
        return Ok(());
    }

    let cmd = args[1].as_str();

    // Determine connection type: Unix socket (default) or TCP
    let socket_path = env::var("DD_PM_GRPC_SOCKET")
        .or_else(|_| env::var("DD_PM_SOCKET")) // Backward compat
        .unwrap_or_else(|_| "/var/run/datadog/process-manager.sock".to_string());
    let use_tcp = env::var("DD_PM_USE_TCP").is_ok();

    let channel = if use_tcp {
        // TCP connection
        let port = env::var("DD_PM_DAEMON_PORT")
            .ok()
            .and_then(|p| p.parse::<u16>().ok())
            .unwrap_or(50051);
        let addr = format!("http://127.0.0.1:{}", port);

        Channel::from_shared(addr)?
            .tcp_keepalive(Some(std::time::Duration::from_secs(60)))
            .tcp_nodelay(true)
            .timeout(std::time::Duration::from_secs(30))
            .connect()
            .await?
    } else {
        // Unix socket connection
        #[cfg(unix)]
        {
            use tokio::net::UnixStream;

            Endpoint::try_from("http://[::]:50051")?
                .connect_with_connector(service_fn(move |_: Uri| {
                    let socket_path = socket_path.clone();
                    async move { UnixStream::connect(socket_path).await }
                }))
                .await?
        }

        #[cfg(not(unix))]
        {
            return Err(
                "Unix sockets are not supported on this platform. Set DD_PM_USE_TCP=1 to use TCP."
                    .into(),
            );
        }
    };

    let mut client = ProcessManagerClient::new(channel);

    // Dispatch to command handlers
    match cmd {
        "create" => commands::handle_create(&mut client, &args).await?,
        "start" => commands::handle_start(&mut client, &args).await?,
        "stop" => commands::handle_stop(&mut client, &args).await?,
        "delete" | "remove" => commands::handle_delete(&mut client, &args).await?,
        "list" => commands::handle_list(&mut client).await?,
        "describe" => commands::handle_describe(&mut client, &args).await?,
        "stats" => commands::handle_stats(&mut client, &args).await?,
        "reload-config" => commands::handle_reload_config(&mut client).await?,
        "update" => commands::handle_update(&mut client, &args).await?,
        "health" => commands::handle_health(&mut client, &args).await?,
        "status" => commands::handle_status(&mut client).await?,
        _ => {
            eprintln!("unknown command: {}", cmd);
            print_usage();
        }
    }

    Ok(())
}

fn print_usage() {
    eprintln!("Process Manager CLI");
    eprintln!();
    eprintln!("Usage: cli <command> [args...]");
    eprintln!();
    eprintln!("Commands:");
    eprintln!("  create <name> <command> [args...]  Create a new process");
    eprintln!("  start <id>                         Start a process");
    eprintln!("  stop <id>                          Stop a process");
    eprintln!("  delete <id> [--force]              Delete a process (aliases: remove)");
    eprintln!("  list                               List all processes");
    eprintln!("  describe <id>                      Show detailed process information");
    eprintln!("  stats <id>                         Show resource usage statistics");
    eprintln!("  update <id> [options...]           Update process configuration");
    eprintln!("  reload-config                      Reload configuration file");
    eprintln!("  health [--wait] [--timeout <secs>] Check daemon health (exit 0 if healthy)");
    eprintln!("  status                             Show detailed daemon status");
    eprintln!();
    eprintln!("Environment Variables:");
    eprintln!(
        "  DD_PM_SOCKET        Unix socket path (default: /var/run/datadog/process-manager.sock)"
    );
    eprintln!("  DD_PM_USE_TCP       Set to use TCP instead of Unix socket");
    eprintln!("  DD_PM_DAEMON_PORT   TCP port when using TCP (default: 50051)");
}
