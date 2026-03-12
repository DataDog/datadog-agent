// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use clap::{Parser, Subcommand};
use dd_procmgrd::grpc::proto;
use dd_procmgrd::grpc::proto::process_manager_client::ProcessManagerClient;
use dd_procmgrd::grpc::server::socket_path;
use std::collections::HashMap;
use std::path::PathBuf;
use std::process::ExitCode;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

#[derive(Parser)]
#[command(name = "dd-procmgr", about = "CLI for dd-procmgrd process manager")]
struct Cli {
    /// Override the daemon socket path (default: DD_PM_SOCKET_PATH or /var/run/datadog/dd-procmgrd.sock)
    #[arg(long, global = true)]
    socket: Option<String>,

    /// Output JSON instead of human-readable format
    #[arg(long, global = true)]
    json: bool,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
#[allow(clippy::large_enum_variant)]
enum Commands {
    /// List all managed processes
    List,
    /// Show detailed information about a process
    Describe {
        /// Process name or UUID
        name_or_uuid: String,
    },
    /// Show daemon status summary
    Status,
    /// Show daemon configuration info
    Config,
    /// Start a process
    Start {
        /// Process name or UUID
        name_or_uuid: String,
    },
    /// Stop a process
    Stop {
        /// Process name or UUID
        name_or_uuid: String,
    },
    /// Create a new process definition
    Create {
        /// Process name
        #[arg(long)]
        name: String,
        /// Executable path
        #[arg(long)]
        command: String,
        /// Command arguments (repeatable)
        #[arg(long, num_args = 1..)]
        args: Vec<String>,
        /// Environment variable KEY=VALUE (repeatable)
        #[arg(long, value_name = "KEY=VALUE")]
        env: Vec<String>,
        /// Working directory
        #[arg(long)]
        working_dir: Option<String>,
        /// Stdout redirect (path or "inherit")
        #[arg(long)]
        stdout: Option<String>,
        /// Stderr redirect (path or "inherit")
        #[arg(long)]
        stderr: Option<String>,
        /// Restart policy: never, always, on-failure, on-success
        #[arg(long)]
        restart_policy: Option<String>,
        /// Human-readable description
        #[arg(long)]
        description: Option<String>,
        /// Start only if this path exists
        #[arg(long)]
        condition_path_exists: Option<String>,
        /// Start the process automatically (default)
        #[arg(long, default_value_t = true, overrides_with = "no_auto_start")]
        auto_start: bool,
        /// Do not start the process automatically
        #[arg(long)]
        no_auto_start: bool,
        /// Start after this process (repeatable)
        #[arg(long)]
        after: Vec<String>,
        /// Start before this process (repeatable)
        #[arg(long)]
        before: Vec<String>,
    },
    /// Reload daemon configuration from disk
    Reload,
}

#[tokio::main]
async fn main() -> ExitCode {
    let cli = Cli::parse();
    match run(cli).await {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("Error: {e}");
            ExitCode::FAILURE
        }
    }
}

async fn connect(socket_override: Option<&str>) -> Result<ProcessManagerClient<Channel>, String> {
    let path: PathBuf = match socket_override {
        Some(s) => PathBuf::from(s),
        None => socket_path(),
    };
    let path_clone = path.clone();
    let channel = Endpoint::try_from("http://[::]:50051")
        .map_err(|e| format!("invalid endpoint: {e}"))?
        .connect_with_connector(service_fn(move |_: Uri| {
            let p = path_clone.clone();
            async move {
                tokio::net::UnixStream::connect(p)
                    .await
                    .map(hyper_util::rt::TokioIo::new)
            }
        }))
        .await
        .map_err(|e| format!("failed to connect to {}: {e}", path.display()))?;
    Ok(ProcessManagerClient::new(channel))
}

async fn run(cli: Cli) -> Result<(), String> {
    let mut client = connect(cli.socket.as_deref()).await?;
    let json = cli.json;

    match cli.command {
        Commands::List => cmd_list(&mut client, json).await,
        Commands::Describe { name_or_uuid } => cmd_describe(&mut client, &name_or_uuid, json).await,
        Commands::Status => cmd_status(&mut client, json).await,
        Commands::Config => cmd_config(&mut client, json).await,
        Commands::Start { name_or_uuid } => cmd_start(&mut client, &name_or_uuid, json).await,
        Commands::Stop { name_or_uuid } => cmd_stop(&mut client, &name_or_uuid, json).await,
        Commands::Create {
            name,
            command,
            args,
            env,
            working_dir,
            stdout,
            stderr,
            restart_policy,
            description,
            condition_path_exists,
            auto_start,
            no_auto_start,
            after,
            before,
        } => {
            let env_map = parse_env_args(&env)?;
            let effective_auto_start = if no_auto_start { false } else { auto_start };
            cmd_create(
                &mut client,
                json,
                &name,
                &command,
                &args,
                env_map,
                working_dir.as_deref(),
                stdout.as_deref(),
                stderr.as_deref(),
                restart_policy.as_deref(),
                description.as_deref(),
                condition_path_exists.as_deref(),
                effective_auto_start,
                &after,
                &before,
            )
            .await
        }
        Commands::Reload => cmd_reload(&mut client, json).await,
    }
}

fn parse_env_args(args: &[String]) -> Result<HashMap<String, String>, String> {
    let mut map = HashMap::new();
    for arg in args {
        let (k, v) = arg
            .split_once('=')
            .ok_or_else(|| format!("invalid --env format '{arg}': expected KEY=VALUE"))?;
        map.insert(k.to_string(), v.to_string());
    }
    Ok(map)
}

fn grpc_err(e: tonic::Status) -> String {
    format!("{}: {}", e.code(), e.message())
}

fn state_name(val: i32) -> &'static str {
    match proto::ProcessState::try_from(val) {
        Ok(proto::ProcessState::Unknown) => "Unknown",
        Ok(proto::ProcessState::Created) => "Created",
        Ok(proto::ProcessState::Starting) => "Starting",
        Ok(proto::ProcessState::Running) => "Running",
        Ok(proto::ProcessState::Stopping) => "Stopping",
        Ok(proto::ProcessState::Stopped) => "Stopped",
        Ok(proto::ProcessState::Crashed) => "Crashed",
        Ok(proto::ProcessState::Exited) => "Exited",
        Ok(proto::ProcessState::Failed) => "Failed",
        Err(_) => "Unknown",
    }
}

fn short_uuid(uuid: &str) -> &str {
    if uuid.len() >= 8 { &uuid[..8] } else { uuid }
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

async fn cmd_list(client: &mut ProcessManagerClient<Channel>, json: bool) -> Result<(), String> {
    let resp = client
        .list(proto::ListRequest {})
        .await
        .map_err(grpc_err)?
        .into_inner();

    if json {
        let items: Vec<serde_json::Value> = resp
            .processes
            .iter()
            .map(|p| {
                serde_json::json!({
                    "uuid": p.uuid,
                    "name": p.name,
                    "state": state_name(p.state),
                    "pid": p.pid,
                    "command": p.command,
                    "args": p.args,
                })
            })
            .collect();
        println!("{}", serde_json::to_string_pretty(&items).unwrap());
        return Ok(());
    }

    if resp.processes.is_empty() {
        println!("No processes");
        return Ok(());
    }

    let rows: Vec<[String; 5]> = resp
        .processes
        .iter()
        .map(|p| {
            [
                p.name.clone(),
                short_uuid(&p.uuid).to_string(),
                state_name(p.state).to_string(),
                if p.pid > 0 {
                    p.pid.to_string()
                } else {
                    "-".to_string()
                },
                p.command.clone(),
            ]
        })
        .collect();

    let headers = ["NAME", "UUID", "STATE", "PID", "COMMAND"];
    let widths: Vec<usize> = (0..5)
        .map(|col| {
            rows.iter()
                .map(|r| r[col].len())
                .max()
                .unwrap_or(0)
                .max(headers[col].len())
        })
        .collect();

    println!(
        "{:<w0$}  {:<w1$}  {:<w2$}  {:<w3$}  {}",
        headers[0],
        headers[1],
        headers[2],
        headers[3],
        headers[4],
        w0 = widths[0],
        w1 = widths[1],
        w2 = widths[2],
        w3 = widths[3],
    );
    for row in &rows {
        println!(
            "{:<w0$}  {:<w1$}  {:<w2$}  {:<w3$}  {}",
            row[0],
            row[1],
            row[2],
            row[3],
            row[4],
            w0 = widths[0],
            w1 = widths[1],
            w2 = widths[2],
            w3 = widths[3],
        );
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// describe
// ---------------------------------------------------------------------------

async fn cmd_describe(
    client: &mut ProcessManagerClient<Channel>,
    name_or_uuid: &str,
    json: bool,
) -> Result<(), String> {
    let resp = client
        .describe(proto::DescribeRequest {
            name_or_uuid: name_or_uuid.to_string(),
        })
        .await
        .map_err(grpc_err)?
        .into_inner();

    let detail = resp.detail.ok_or("no detail returned")?;

    if json {
        let val = serde_json::json!({
            "uuid": detail.uuid,
            "name": detail.name,
            "description": detail.description,
            "state": state_name(detail.state),
            "pid": detail.pid,
            "command": detail.command,
            "args": detail.args,
            "working_dir": detail.working_dir,
            "env": detail.env,
            "restart_policy": detail.restart_policy,
            "auto_start": detail.auto_start,
            "stdout": detail.stdout,
            "stderr": detail.stderr,
            "condition_path_exists": detail.condition_path_exists,
            "after": detail.after,
            "before": detail.before,
        });
        println!("{}", serde_json::to_string_pretty(&val).unwrap());
        return Ok(());
    }

    let pid_str = if detail.pid > 0 {
        detail.pid.to_string()
    } else {
        "-".to_string()
    };
    let args_str = if detail.args.is_empty() {
        "-".to_string()
    } else {
        detail.args.join(" ")
    };

    println!("Name:                {}", detail.name);
    println!("UUID:                {}", detail.uuid);
    println!("State:               {}", state_name(detail.state));
    println!("PID:                 {}", pid_str);
    println!("Command:             {}", detail.command);
    println!("Args:                {}", args_str);
    if !detail.description.is_empty() {
        println!("Description:         {}", detail.description);
    }
    if !detail.working_dir.is_empty() {
        println!("Working Dir:         {}", detail.working_dir);
    }
    println!("Restart Policy:      {}", detail.restart_policy);
    println!("Auto Start:          {}", detail.auto_start);
    println!("Stdout:              {}", detail.stdout);
    println!("Stderr:              {}", detail.stderr);
    if !detail.condition_path_exists.is_empty() {
        println!("Condition Path:      {}", detail.condition_path_exists);
    }
    if !detail.after.is_empty() {
        println!("After:               [{}]", detail.after.join(", "));
    }
    if !detail.before.is_empty() {
        println!("Before:              [{}]", detail.before.join(", "));
    }
    if !detail.env.is_empty() {
        let mut keys: Vec<&String> = detail.env.keys().collect();
        keys.sort();
        println!("Environment:");
        for k in keys {
            println!("  {}={}", k, detail.env[k]);
        }
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

async fn cmd_status(client: &mut ProcessManagerClient<Channel>, json: bool) -> Result<(), String> {
    let resp = client
        .get_status(proto::GetStatusRequest {})
        .await
        .map_err(grpc_err)?
        .into_inner();

    if json {
        let val = serde_json::json!({
            "version": resp.version,
            "ready": resp.ready,
            "uptime_seconds": resp.uptime_seconds,
            "total_processes": resp.total_processes,
            "running_processes": resp.running_processes,
            "stopped_processes": resp.stopped_processes,
            "created_processes": resp.created_processes,
            "failed_processes": resp.failed_processes,
            "exited_processes": resp.exited_processes,
            "starting_processes": resp.starting_processes,
            "stopping_processes": resp.stopping_processes,
        });
        println!("{}", serde_json::to_string_pretty(&val).unwrap());
        return Ok(());
    }

    let uptime = format_duration(resp.uptime_seconds);
    println!("Version:             {}", resp.version);
    println!("Ready:               {}", resp.ready);
    println!("Uptime:              {}", uptime);
    println!("Total Processes:     {}", resp.total_processes);
    println!("  Running:           {}", resp.running_processes);
    println!("  Stopped:           {}", resp.stopped_processes);
    println!("  Created:           {}", resp.created_processes);
    println!("  Failed:            {}", resp.failed_processes);
    println!("  Exited:            {}", resp.exited_processes);
    if resp.starting_processes > 0 {
        println!("  Starting:          {}", resp.starting_processes);
    }
    if resp.stopping_processes > 0 {
        println!("  Stopping:          {}", resp.stopping_processes);
    }
    Ok(())
}

fn format_duration(secs: u64) -> String {
    let hours = secs / 3600;
    let mins = (secs % 3600) / 60;
    let s = secs % 60;
    if hours > 0 {
        format!("{hours}h {mins}m {s}s")
    } else if mins > 0 {
        format!("{mins}m {s}s")
    } else {
        format!("{s}s")
    }
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

async fn cmd_config(client: &mut ProcessManagerClient<Channel>, json: bool) -> Result<(), String> {
    let resp = client
        .get_config(proto::GetConfigRequest {})
        .await
        .map_err(grpc_err)?
        .into_inner();

    if json {
        let val = serde_json::json!({
            "source": resp.source,
            "location": resp.location,
            "loaded_processes": resp.loaded_processes,
            "runtime_processes": resp.runtime_processes,
        });
        println!("{}", serde_json::to_string_pretty(&val).unwrap());
        return Ok(());
    }

    println!("Source:              {}", resp.source);
    println!("Location:            {}", resp.location);
    println!("Loaded Processes:    {}", resp.loaded_processes);
    println!("Runtime Processes:   {}", resp.runtime_processes);
    Ok(())
}

// ---------------------------------------------------------------------------
// start
// ---------------------------------------------------------------------------

async fn cmd_start(
    client: &mut ProcessManagerClient<Channel>,
    name_or_uuid: &str,
    json: bool,
) -> Result<(), String> {
    let resp = client
        .start(proto::StartRequest {
            name_or_uuid: name_or_uuid.to_string(),
        })
        .await
        .map_err(grpc_err)?
        .into_inner();

    if json {
        let val = serde_json::json!({
            "uuid": resp.uuid,
            "pid": resp.pid,
            "state": state_name(resp.state),
        });
        println!("{}", serde_json::to_string_pretty(&val).unwrap());
        return Ok(());
    }

    println!("{}", name_or_uuid);
    println!("  UUID:   {}", resp.uuid);
    println!("  State:  {}", state_name(resp.state));
    if resp.pid > 0 {
        println!("  PID:    {}", resp.pid);
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// stop
// ---------------------------------------------------------------------------

async fn cmd_stop(
    client: &mut ProcessManagerClient<Channel>,
    name_or_uuid: &str,
    json: bool,
) -> Result<(), String> {
    let resp = client
        .stop(proto::StopRequest {
            name_or_uuid: name_or_uuid.to_string(),
        })
        .await
        .map_err(grpc_err)?
        .into_inner();

    if json {
        let val = serde_json::json!({
            "uuid": resp.uuid,
            "state": state_name(resp.state),
        });
        println!("{}", serde_json::to_string_pretty(&val).unwrap());
        return Ok(());
    }

    println!("{}", name_or_uuid);
    println!("  UUID:   {}", resp.uuid);
    println!("  State:  {}", state_name(resp.state));
    Ok(())
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

#[allow(clippy::too_many_arguments)]
async fn cmd_create(
    client: &mut ProcessManagerClient<Channel>,
    json: bool,
    name: &str,
    command: &str,
    args: &[String],
    env: HashMap<String, String>,
    working_dir: Option<&str>,
    stdout: Option<&str>,
    stderr: Option<&str>,
    restart_policy: Option<&str>,
    description: Option<&str>,
    condition_path_exists: Option<&str>,
    auto_start: bool,
    after: &[String],
    before: &[String],
) -> Result<(), String> {
    let resp = client
        .create(proto::CreateRequest {
            name: name.to_string(),
            command: command.to_string(),
            args: args.to_vec(),
            env,
            working_dir: working_dir.unwrap_or_default().to_string(),
            stdout: stdout.unwrap_or_default().to_string(),
            stderr: stderr.unwrap_or_default().to_string(),
            restart_policy: restart_policy.unwrap_or_default().to_string(),
            description: description.unwrap_or_default().to_string(),
            condition_path_exists: condition_path_exists.unwrap_or_default().to_string(),
            auto_start: Some(auto_start),
            after: after.to_vec(),
            before: before.to_vec(),
        })
        .await
        .map_err(grpc_err)?
        .into_inner();

    if json {
        let val = serde_json::json!({
            "name": name,
            "uuid": resp.uuid,
        });
        println!("{}", serde_json::to_string_pretty(&val).unwrap());
        return Ok(());
    }

    println!("{}", name);
    println!("  UUID:   {}", resp.uuid);
    Ok(())
}

// ---------------------------------------------------------------------------
// reload
// ---------------------------------------------------------------------------

async fn cmd_reload(client: &mut ProcessManagerClient<Channel>, json: bool) -> Result<(), String> {
    let resp = client
        .reload_config(proto::ReloadConfigRequest {})
        .await
        .map_err(grpc_err)?
        .into_inner();

    if json {
        let val = serde_json::json!({
            "added": resp.added,
            "removed": resp.removed,
            "modified": resp.modified,
            "unchanged": resp.unchanged,
        });
        println!("{}", serde_json::to_string_pretty(&val).unwrap());
        return Ok(());
    }

    if !resp.added.is_empty() {
        println!("Added:     {}", resp.added.join(", "));
    }
    if !resp.removed.is_empty() {
        println!("Removed:   {}", resp.removed.join(", "));
    }
    if !resp.modified.is_empty() {
        println!("Modified:  {}", resp.modified.join(", "));
    }
    if !resp.unchanged.is_empty() {
        println!("Unchanged: {}", resp.unchanged.join(", "));
    }
    if resp.added.is_empty()
        && resp.removed.is_empty()
        && resp.modified.is_empty()
        && resp.unchanged.is_empty()
    {
        println!("No changes");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_state_name_all_variants() {
        assert_eq!(state_name(proto::ProcessState::Unknown as i32), "Unknown");
        assert_eq!(state_name(proto::ProcessState::Created as i32), "Created");
        assert_eq!(
            state_name(proto::ProcessState::Starting as i32),
            "Starting"
        );
        assert_eq!(state_name(proto::ProcessState::Running as i32), "Running");
        assert_eq!(
            state_name(proto::ProcessState::Stopping as i32),
            "Stopping"
        );
        assert_eq!(state_name(proto::ProcessState::Stopped as i32), "Stopped");
        assert_eq!(state_name(proto::ProcessState::Crashed as i32), "Crashed");
        assert_eq!(state_name(proto::ProcessState::Exited as i32), "Exited");
        assert_eq!(state_name(proto::ProcessState::Failed as i32), "Failed");
    }

    #[test]
    fn test_state_name_invalid() {
        assert_eq!(state_name(9999), "Unknown");
        assert_eq!(state_name(-1), "Unknown");
    }

    #[test]
    fn test_short_uuid_normal() {
        let uuid = "550e8400-e29b-41d4-a716-446655440000";
        assert_eq!(short_uuid(uuid), "550e8400");
    }

    #[test]
    fn test_short_uuid_exactly_8() {
        assert_eq!(short_uuid("abcdefgh"), "abcdefgh");
    }

    #[test]
    fn test_short_uuid_shorter_than_8() {
        assert_eq!(short_uuid("abc"), "abc");
    }

    #[test]
    fn test_short_uuid_empty() {
        assert_eq!(short_uuid(""), "");
    }

    #[test]
    fn test_format_duration_hours_mins_secs() {
        assert_eq!(format_duration(3 * 3600 + 24 * 60 + 15), "3h 24m 15s");
    }

    #[test]
    fn test_format_duration_mins_secs() {
        assert_eq!(format_duration(5 * 60 + 30), "5m 30s");
    }

    #[test]
    fn test_format_duration_secs_only() {
        assert_eq!(format_duration(42), "42s");
    }

    #[test]
    fn test_format_duration_zero() {
        assert_eq!(format_duration(0), "0s");
    }

    #[test]
    fn test_format_duration_exact_hour() {
        assert_eq!(format_duration(3600), "1h 0m 0s");
    }

    #[test]
    fn test_parse_env_args_valid() {
        let args = vec!["KEY=value".to_string(), "FOO=bar".to_string()];
        let map = parse_env_args(&args).unwrap();
        assert_eq!(map.get("KEY").unwrap(), "value");
        assert_eq!(map.get("FOO").unwrap(), "bar");
    }

    #[test]
    fn test_parse_env_args_value_with_equals() {
        let args = vec!["PATH=/usr/bin:/usr/local/bin".to_string()];
        let map = parse_env_args(&args).unwrap();
        assert_eq!(map.get("PATH").unwrap(), "/usr/bin:/usr/local/bin");
    }

    #[test]
    fn test_parse_env_args_empty() {
        let args: Vec<String> = vec![];
        let map = parse_env_args(&args).unwrap();
        assert!(map.is_empty());
    }

    #[test]
    fn test_parse_env_args_missing_equals() {
        let args = vec!["INVALID".to_string()];
        let err = parse_env_args(&args).unwrap_err();
        assert!(err.contains("INVALID"));
        assert!(err.contains("KEY=VALUE"));
    }
}
