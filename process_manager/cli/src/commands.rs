use crate::formatters::{format_state, format_timestamp};
use crate::options::{parse_cpu, parse_memory, CreateOptions};
use crate::process_manager;
use crate::process_manager::process_manager_client::ProcessManagerClient;
use crate::process_manager::ProcessState;
use tonic::transport::Channel;

/// Parse process type string to protobuf enum
fn parse_process_type(type_str: Option<&str>) -> i32 {
    match type_str {
        Some("simple") | None => process_manager::ProcessType::Simple as i32,
        Some("forking") => process_manager::ProcessType::Forking as i32,
        Some("oneshot") => process_manager::ProcessType::Oneshot as i32,
        Some("notify") => process_manager::ProcessType::Notify as i32,
        Some(_) => process_manager::ProcessType::Simple as i32, // Default for invalid types
    }
}

fn build_resource_limits(
    opts: &CreateOptions,
) -> Result<Option<process_manager::ResourceLimits>, String> {
    // Check if any resource limits are specified
    if opts.cpu_request.is_none()
        && opts.cpu_limit.is_none()
        && opts.memory_request.is_none()
        && opts.memory_limit.is_none()
        && opts.pids_limit.is_none()
        && opts.oom_score_adj.is_none()
    {
        return Ok(None);
    }

    // Parse CPU values
    let cpu_request = if let Some(ref val) = opts.cpu_request {
        Some(parse_cpu(val)?)
    } else {
        None
    };

    let cpu_limit = if let Some(ref val) = opts.cpu_limit {
        Some(parse_cpu(val)?)
    } else {
        None
    };

    // Parse memory values
    let memory_request = if let Some(ref val) = opts.memory_request {
        Some(parse_memory(val)?)
    } else {
        None
    };

    let memory_limit = if let Some(ref val) = opts.memory_limit {
        Some(parse_memory(val)?)
    } else {
        None
    };

    Ok(Some(process_manager::ResourceLimits {
        cpu_request: cpu_request.unwrap_or(0),
        cpu_limit: cpu_limit.unwrap_or(0),
        memory_request: memory_request.unwrap_or(0),
        memory_limit: memory_limit.unwrap_or(0),
        pids_limit: opts.pids_limit.unwrap_or(0),
        oom_score_adj: opts.oom_score_adj.unwrap_or(0),
    }))
}

fn build_health_check(
    opts: &CreateOptions,
) -> Result<Option<process_manager::HealthCheck>, String> {
    // If no health check type specified, no health check
    let check_type_str = match &opts.health_check_type {
        Some(t) => t,
        None => return Ok(None),
    };

    // Parse health check type
    let check_type = match check_type_str.as_str() {
        "http" => process_manager::HealthCheckType::Http as i32,
        "tcp" => process_manager::HealthCheckType::Tcp as i32,
        "exec" => process_manager::HealthCheckType::Exec as i32,
        _ => return Err(format!("Invalid health check type: {}", check_type_str)),
    };

    // Build health check message
    let health_check = process_manager::HealthCheck {
        r#type: check_type,
        interval: opts.health_check_interval.unwrap_or(30),
        timeout: opts.health_check_timeout.unwrap_or(5),
        retries: opts.health_check_retries.unwrap_or(3),
        start_period: opts.health_check_start_period.unwrap_or(0),
        restart_after: 0, // Default: never restart on health failure (informational only)
        // HTTP fields
        http_endpoint: opts.health_check_http_endpoint.clone().unwrap_or_default(),
        http_method: opts.health_check_http_method.clone().unwrap_or_default(),
        http_expected_status: opts.health_check_http_status.map(|s| s as u32).unwrap_or(0),
        // TCP fields
        tcp_host: opts.health_check_tcp_host.clone().unwrap_or_default(),
        tcp_port: opts.health_check_tcp_port.map(|p| p as u32).unwrap_or(0),
        // Exec fields
        exec_command: opts.health_check_exec_command.clone().unwrap_or_default(),
        exec_args: opts.health_check_exec_args.clone(),
    };

    // Validate required fields per type
    match check_type_str.as_str() {
        "http" => {
            if health_check.http_endpoint.is_empty() {
                return Err("HTTP health check requires --health-check-http-endpoint".to_string());
            }
        }
        "tcp" => {
            if health_check.tcp_host.is_empty() {
                return Err("TCP health check requires --health-check-tcp-host".to_string());
            }
            if health_check.tcp_port == 0 {
                return Err("TCP health check requires --health-check-tcp-port".to_string());
            }
        }
        "exec" => {
            if health_check.exec_command.is_empty() {
                return Err("Exec health check requires --health-check-exec-command".to_string());
            }
        }
        _ => {}
    }

    Ok(Some(health_check))
}

pub async fn handle_create(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    // Parse command-line options
    let opts = match CreateOptions::parse(args) {
        Ok(o) => o,
        Err(e) => {
            eprintln!("Error: {}", e);
            eprintln!();
            CreateOptions::print_usage();
            return Ok(());
        }
    };

    // Map restart policy string to enum
    let restart_enum = if let Some(ref restart) = opts.restart {
        match restart.to_lowercase().as_str() {
            "never" => process_manager::RestartPolicy::Never,
            "always" => process_manager::RestartPolicy::Always,
            "on-failure" => process_manager::RestartPolicy::OnFailure,
            "on-success" => process_manager::RestartPolicy::OnSuccess,
            _ => {
                eprintln!("Invalid restart policy: {}. Using 'never'.", restart);
                process_manager::RestartPolicy::Never
            }
        }
    } else {
        process_manager::RestartPolicy::Never
    };

    // Map kill_signal string to enum
    let kill_signal_enum = if let Some(ref kill_signal) = opts.kill_signal {
        match kill_signal.to_uppercase().as_str() {
            "SIGTERM" => process_manager::KillSignal::Sigterm,
            "SIGINT" => process_manager::KillSignal::Sigint,
            "SIGQUIT" => process_manager::KillSignal::Sigquit,
            "SIGKILL" => process_manager::KillSignal::Sigkill,
            "SIGHUP" => process_manager::KillSignal::Sighup,
            "SIGUSR1" => process_manager::KillSignal::Sigusr1,
            "SIGUSR2" => process_manager::KillSignal::Sigusr2,
            _ => {
                eprintln!("Invalid kill signal: {}. Using SIGTERM.", kill_signal);
                process_manager::KillSignal::Sigterm
            }
        }
    } else {
        process_manager::KillSignal::Sigterm
    };

    // Map kill_mode string to enum
    let kill_mode_enum = if let Some(ref kill_mode) = opts.kill_mode {
        match kill_mode.to_lowercase().as_str() {
            "control-group" | "controlgroup" | "cgroup" => process_manager::KillMode::ControlGroup,
            "process-group" | "processgroup" => process_manager::KillMode::ProcessGroup,
            "process" => process_manager::KillMode::Process,
            "mixed" => process_manager::KillMode::Mixed,
            _ => {
                eprintln!("Invalid kill mode: {}. Using control-group.", kill_mode);
                process_manager::KillMode::ControlGroup
            }
        }
    } else {
        process_manager::KillMode::ControlGroup
    };

    let req = tonic::Request::new(process_manager::CreateRequest {
        name: opts.name.clone(),
        command: opts.command.clone(),
        args: opts.args.clone(),
        restart: restart_enum as i32,
        restart_sec: opts.restart_sec.unwrap_or(0),
        restart_max_delay: opts.restart_max_delay.unwrap_or(0),
        start_limit_burst: opts.start_limit_burst.unwrap_or(0),
        start_limit_interval: opts.start_limit_interval.unwrap_or(0),
        working_dir: opts.working_dir.clone().unwrap_or_default(),
        env: opts.env_vars.clone(),
        environment_file: opts.environment_file.clone().unwrap_or_default(),
        pidfile: opts.pidfile.clone().unwrap_or_default(),
        stdout: opts.stdout.clone().unwrap_or_default(),
        stderr: opts.stderr.clone().unwrap_or_default(),
        timeout_start_sec: opts.timeout_start_sec.unwrap_or(0),
        timeout_stop_sec: opts.timeout_stop_sec.unwrap_or(0),
        kill_signal: kill_signal_enum as i32,
        kill_mode: kill_mode_enum as i32,
        success_exit_status: opts.success_exit_status.clone().unwrap_or_default(),
        exec_start_pre: opts.exec_start_pre.clone(),
        exec_start_post: opts.exec_start_post.clone(),
        exec_stop_post: opts.exec_stop_post.clone(),
        user: opts.user.clone().unwrap_or_default(),
        group: opts.group.clone().unwrap_or_default(),
        auto_start: opts.auto_start,
        // Dependencies
        after: opts.after.clone(),
        before: opts.before.clone(),
        requires: opts.requires.clone(),
        wants: opts.wants.clone(),
        binds_to: opts.binds_to.clone(),
        conflicts: opts.conflicts.clone(),
        // Process type
        process_type: parse_process_type(opts.process_type.as_deref()),
        // Health check
        health_check: build_health_check(&opts)?,
        // Resource limits
        resource_limits: build_resource_limits(&opts)?,
        // Conditional execution
        condition_path_exists: opts.condition_path_exists.clone(),
        // Runtime directories
        runtime_directory: opts.runtime_directory.clone(),
        // Ambient capabilities (Linux-only)
        ambient_capabilities: opts.ambient_capabilities.clone(),
    });

    let resp = client.create(req).await?;
    let inner = resp.into_inner();

    println!("[OK] Process created");
    println!("  Name: {}", opts.name);
    println!("  ID: {}", inner.id);
    println!(
        "  Command: {}{}",
        opts.command,
        if !opts.args.is_empty() {
            format!(" {}", opts.args.join(" "))
        } else {
            String::new()
        }
    );
    println!(
        "  State: {}",
        format_state(ProcessState::from_i32(inner.state).unwrap_or(ProcessState::Unknown))
    );

    if opts.auto_start {
        println!("  Auto-started!");
    }

    Ok(())
}

pub async fn handle_start(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    if args.len() != 3 {
        eprintln!("usage: cli start <id>");
        return Ok(());
    }

    let id = args[2].clone();
    let req = tonic::Request::new(process_manager::StartRequest { id: id.clone() });

    match client.start(req).await {
        Ok(resp) => {
            let inner = resp.into_inner();
            println!("[OK] Process started");
            println!("  ID: {}", id);
            println!(
                "  State: {}",
                format_state(ProcessState::from_i32(inner.state).unwrap_or(ProcessState::Unknown))
            );
            Ok(())
        }
        Err(e) => {
            eprintln!("[ERROR] {}", e);
            Err(e.into())
        }
    }
}

pub async fn handle_stop(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    if args.len() != 3 {
        eprintln!("usage: cli stop <id>");
        return Ok(());
    }

    let id = args[2].clone();
    let req = tonic::Request::new(process_manager::StopRequest { id: id.clone() });

    let resp = client.stop(req).await?;
    let inner = resp.into_inner();

    println!("[OK] Stop signal sent");
    println!("  ID: {}", id);
    println!(
        "  State: {}",
        format_state(ProcessState::from_i32(inner.state).unwrap_or(ProcessState::Unknown))
    );
    Ok(())
}

pub async fn handle_delete(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    if args.len() < 3 {
        eprintln!("usage: cli delete <id> [--force]");
        eprintln!("  --force: Delete even if process is running (stops it first)");
        return Ok(());
    }

    let id = args[2].clone();
    let force = args.len() > 3 && args[3] == "--force";

    let req = tonic::Request::new(process_manager::DeleteRequest {
        id: id.clone(),
        force,
    });

    client.delete(req).await?;

    println!("[OK] Process deleted");
    println!("  ID: {}", id);
    Ok(())
}

pub async fn handle_list(
    client: &mut ProcessManagerClient<Channel>,
) -> Result<(), Box<dyn std::error::Error>> {
    let req = tonic::Request::new(process_manager::ListRequest {});
    let resp = client.list(req).await?;
    let body = resp.into_inner();

    if body.processes.is_empty() {
        println!("No processes");
        return Ok(());
    }

    // Header
    println!(
        "{:<25}  {:<36}  {:<5}  {:<7}  {:<4}  {:<24}  {:<19}  {:<19}  {:<19}  {:<5}",
        "NAME", "ID", "PID", "STATE", "RUNS", "COMMAND", "CREATED", "STARTED", "ENDED", "EXIT"
    );
    println!(
        "{:-<25}  {:-<36}  {:-<5}  {:-<7}  {:-<4}  {:-<24}  {:-<19}  {:-<19}  {:-<19}  {:-<5}",
        "", "", "", "", "", "", "", "", "", ""
    );

    for p in &body.processes {
        let state = ProcessState::from_i32(p.state).unwrap_or(ProcessState::Unknown);

        let pid_str: String = if p.pid == 0 {
            "-".into()
        } else {
            p.pid.to_string()
        };

        let created = format_timestamp(p.created_at);
        let started = format_timestamp(p.started_at);
        let ended = format_timestamp(p.ended_at);

        let exit_info = if p.exit_code != 0 {
            p.exit_code.to_string()
        } else if !p.signal.is_empty() {
            p.signal.clone()
        } else {
            "-".into()
        };

        // Truncate command if too long
        let mut command = p.command.clone();
        if command.len() > 24 {
            command.truncate(21);
            command.push_str("...");
        }

        println!(
            "{:<25}  {:<36}  {:<5}  {:<7}  {:<4}  {:<24}  {:<19}  {:<19}  {:<19}  {:<5}",
            p.name,
            p.id,
            pid_str,
            format_state(state),
            p.run_count,
            command,
            created,
            started,
            ended,
            exit_info
        );
    }

    println!();
    Ok(())
}

pub async fn handle_describe(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    if args.len() != 3 {
        eprintln!("usage: cli describe <id>");
        return Ok(());
    }

    let id = args[2].clone();
    let req = tonic::Request::new(process_manager::DescribeRequest { id: id.clone() });

    let resp = client.describe(req).await?;
    let inner = resp.into_inner();

    let detail = inner.detail.unwrap();
    let state = ProcessState::from_i32(detail.state).unwrap_or(ProcessState::Unknown);

    // Header
    println!("\nProcess Details");
    println!("{}", "─".repeat(60));

    // Basic Info
    println!("\nBASIC INFORMATION");
    println!("  Name:            {}", detail.name);
    println!("  ID:              {}", detail.id);
    println!("  Command:         {}", detail.command);
    if !detail.args.is_empty() {
        println!("  Arguments:       {}", detail.args.join(" "));
    }
    println!("  State:           {}", format_state(state));
    println!("  Restart Policy:  {}", detail.restart_policy);
    println!("  Run Count:       {}", detail.run_count);

    // Runtime Info
    println!("\nRUNTIME INFORMATION");
    if detail.pid != 0 {
        println!("  PID:             {}", detail.pid);
    } else {
        println!("  PID:             -");
    }
    if !detail.health_status.is_empty() {
        println!("  Health Status:   {}", detail.health_status);
        if detail.health_check_failures > 0 {
            println!("  Health Failures: {}", detail.health_check_failures);
        }
        if detail.last_health_check > 0 {
            println!(
                "  Last Check:      {}",
                format_timestamp(detail.last_health_check)
            );
        }
    }

    // Lifecycle
    println!("\nLIFECYCLE");
    println!("  Created:         {}", format_timestamp(detail.created_at));
    println!("  Started:         {}", format_timestamp(detail.started_at));
    println!("  Ended:           {}", format_timestamp(detail.ended_at));

    // Exit Information
    let has_exited = matches!(state, ProcessState::Exited | ProcessState::Crashed);
    if has_exited || detail.exit_code != 0 || !detail.signal.is_empty() {
        println!("\nEXIT INFORMATION");
        if has_exited || detail.exit_code != 0 {
            println!("  Exit Code:       {}", detail.exit_code);
        }
        if !detail.signal.is_empty() {
            println!("  Signal:          {}", detail.signal);
        }
    }

    // Dependencies
    if !detail.requires.is_empty()
        || !detail.wants.is_empty()
        || !detail.binds_to.is_empty()
        || !detail.conflicts.is_empty()
        || !detail.after.is_empty()
        || !detail.before.is_empty()
    {
        println!("\nDEPENDENCIES");
        if !detail.requires.is_empty() {
            println!("  Requires:        {}", detail.requires.join(", "));
        }
        if !detail.wants.is_empty() {
            println!("  Wants:           {}", detail.wants.join(", "));
        }
        if !detail.binds_to.is_empty() {
            println!("  BindsTo:         {}", detail.binds_to.join(", "));
        }
        if !detail.conflicts.is_empty() {
            println!("  Conflicts:       {}", detail.conflicts.join(", "));
        }
        if !detail.after.is_empty() {
            println!("  After:           {}", detail.after.join(", "));
        }
        if !detail.before.is_empty() {
            println!("  Before:          {}", detail.before.join(", "));
        }
    }

    // Configuration
    println!("\nCONFIGURATION");
    if !detail.working_dir.is_empty() {
        println!("  Working Dir:     {}", detail.working_dir);
    }
    if !detail.env.is_empty() {
        println!("  Environment:");
        for (key, value) in &detail.env {
            println!("    {}={}", key, value);
        }
    }

    // Resource Limits
    if let Some(limits) = detail.resource_limits {
        if limits.cpu_limit > 0
            || limits.memory_limit > 0
            || limits.pids_limit > 0
            || limits.cpu_request > 0
            || limits.memory_request > 0
        {
            println!("\nRESOURCE LIMITS");
            if limits.cpu_request > 0 {
                println!("  CPU Request:     {}m", limits.cpu_request);
            }
            if limits.cpu_limit > 0 {
                println!("  CPU Limit:       {}m", limits.cpu_limit);
            }
            if limits.memory_request > 0 {
                let mb = limits.memory_request / (1024 * 1024);
                println!("  Memory Request:  {} MB", mb);
            }
            if limits.memory_limit > 0 {
                let mb = limits.memory_limit / (1024 * 1024);
                println!("  Memory Limit:    {} MB", mb);
            }
            if limits.pids_limit > 0 {
                println!("  PIDs Limit:      {}", limits.pids_limit);
            }
            if limits.oom_score_adj != 0 {
                println!("  OOM Score Adj:   {}", limits.oom_score_adj);
            }
        }
    }

    // Conditional Execution
    if !detail.condition_path_exists.is_empty() {
        println!("\nCONDITIONAL EXECUTION");
        println!("  Conditions:");
        for condition in &detail.condition_path_exists {
            println!("    {}", condition);
        }
    }

    // Runtime Directories
    if !detail.runtime_directory.is_empty() {
        println!("\nRUNTIME DIRECTORIES");
        for dir in &detail.runtime_directory {
            println!("  /run/{}", dir);
        }
    }

    // Ambient Capabilities
    if !detail.ambient_capabilities.is_empty() {
        println!("\nAMBIENT CAPABILITIES");
        for cap in &detail.ambient_capabilities {
            println!("  {}", cap);
        }
    }

    println!();
    Ok(())
}

pub async fn handle_reload_config(
    client: &mut ProcessManagerClient<Channel>,
) -> Result<(), Box<dyn std::error::Error>> {
    let req = tonic::Request::new(process_manager::ReloadConfigRequest {});
    client.reload_config(req).await?;

    println!("[OK] Configuration reloaded");
    Ok(())
}

pub async fn handle_stats(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    if args.len() != 3 {
        eprintln!("usage: cli stats <id>");
        return Ok(());
    }

    let id = args[2].clone();
    let req = tonic::Request::new(process_manager::GetResourceUsageRequest { id: id.clone() });

    let resp = match client.get_resource_usage(req).await {
        Ok(resp) => resp,
        Err(e) => {
            // Handle gracefully when cgroups are unavailable
            if e.message().contains("Resource usage not available") {
                println!("[ERROR] Resource usage not available for process '{}'", id);
                println!("This may occur if:");
                println!("  - cgroups v2 is not enabled on this system");
                println!("  - The process has no resource limits configured");
                println!("  - The process is not currently running");
                return Ok(());
            }
            return Err(e.into());
        }
    };
    let inner = resp.into_inner();

    let usage = inner.usage.unwrap();

    // Header
    println!("\nResource Usage Statistics");
    println!("{}", "─".repeat(60));

    // Memory
    println!("\nMEMORY");
    if usage.memory_limit > 0 {
        let mem_mb = usage.memory_current as f64 / (1024.0 * 1024.0);
        let limit_mb = usage.memory_limit as f64 / (1024.0 * 1024.0);
        let percent = (usage.memory_current as f64 / usage.memory_limit as f64) * 100.0;
        println!("  Current:  {:.2} MB", mem_mb);
        println!("  Limit:    {:.2} MB", limit_mb);
        println!("  Usage:    {:.1}%", percent);
    } else {
        let mem_mb = usage.memory_current as f64 / (1024.0 * 1024.0);
        println!("  Current:  {:.2} MB", mem_mb);
        println!("  Limit:    unlimited");
    }

    // CPU
    println!("\nCPU");
    let cpu_sec = usage.cpu_usage_usec as f64 / 1_000_000.0;
    let user_sec = usage.cpu_user_usec as f64 / 1_000_000.0;
    let system_sec = usage.cpu_system_usec as f64 / 1_000_000.0;
    println!("  Total:    {:.2} seconds", cpu_sec);
    println!("  User:     {:.2} seconds", user_sec);
    println!("  System:   {:.2} seconds", system_sec);

    // PIDs
    println!("\nPROCESSES/THREADS");
    if usage.pids_limit > 0 {
        let percent = (usage.pids_current as f64 / usage.pids_limit as f64) * 100.0;
        println!("  Current:  {}", usage.pids_current);
        println!("  Limit:    {}", usage.pids_limit);
        println!("  Usage:    {:.1}%", percent);
    } else {
        println!("  Current:  {}", usage.pids_current);
        println!("  Limit:    unlimited");
    }

    println!();
    Ok(())
}

pub async fn handle_update(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    if args.len() < 3 {
        eprintln!("Usage: cli update <id> [options...]");
        eprintln!();
        eprintln!("Hot-update options (no restart required):");
        eprintln!("  --restart <never|always|on-failure|on-success>");
        eprintln!("  --timeout-stop-sec <seconds>");
        eprintln!("  --restart-sec <seconds>");
        eprintln!("  --restart-max <seconds>");
        eprintln!("  --cpu-limit <cores>");
        eprintln!("  --memory-limit <bytes>");
        eprintln!("  --pids-limit <count>");
        eprintln!();
        eprintln!("Options requiring restart (use --restart-process to apply):");
        eprintln!("  --env KEY=VALUE");
        eprintln!("  --env-file <path>");
        eprintln!("  --working-dir <path>");
        eprintln!("  --user <username>");
        eprintln!("  --group <groupname>");
        eprintln!("  --runtime-directory <dir>");
        eprintln!("  --ambient-capability <CAP_NAME>");
        eprintln!();
        eprintln!("Flags:");
        eprintln!("  --restart-process   Restart process after update to apply changes");
        eprintln!("  --dry-run           Validate update without applying");
        return Ok(());
    }

    let id = args[2].clone();
    let mut request = process_manager::UpdateRequest {
        id: id.clone(),
        restart_policy: None,
        timeout_stop_sec: None,
        restart_sec: None,
        restart_max: None,
        resource_limits: None,
        health_check: None,
        success_exit_status: vec![],
        env: vec![],
        env_file: None,
        working_dir: None,
        user: None,
        group: None,
        runtime_directory: vec![],
        ambient_capabilities: vec![],
        kill_mode: None,
        kill_signal: None,
        pidfile: None,
        restart_process: false,
        dry_run: false,
    };

    // Parse options
    let mut i = 3;
    while i < args.len() {
        let arg = args[i].as_str();
        match arg {
            "--restart" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --restart");
                    return Ok(());
                }
                request.restart_policy = Some(args[i + 1].clone());
                i += 2;
            }
            "--timeout-stop-sec" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --timeout-stop-sec");
                    return Ok(());
                }
                request.timeout_stop_sec = Some(args[i + 1].parse()?);
                i += 2;
            }
            "--restart-sec" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --restart-sec");
                    return Ok(());
                }
                request.restart_sec = Some(args[i + 1].parse()?);
                i += 2;
            }
            "--restart-max" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --restart-max");
                    return Ok(());
                }
                request.restart_max = Some(args[i + 1].parse()?);
                i += 2;
            }
            "--cpu-limit" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --cpu-limit");
                    return Ok(());
                }
                let cpu_limit = parse_cpu(&args[i + 1])?;
                request.resource_limits = Some(process_manager::ResourceLimits {
                    cpu_limit,
                    cpu_request: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.cpu_request)
                        .unwrap_or(0),
                    memory_limit: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.memory_limit)
                        .unwrap_or(0),
                    memory_request: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.memory_request)
                        .unwrap_or(0),
                    pids_limit: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.pids_limit)
                        .unwrap_or(0),
                    oom_score_adj: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.oom_score_adj)
                        .unwrap_or(0),
                });
                i += 2;
            }
            "--memory-limit" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --memory-limit");
                    return Ok(());
                }
                let memory_limit = parse_memory(&args[i + 1])?;
                request.resource_limits = Some(process_manager::ResourceLimits {
                    memory_limit,
                    cpu_limit: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.cpu_limit)
                        .unwrap_or(0),
                    cpu_request: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.cpu_request)
                        .unwrap_or(0),
                    memory_request: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.memory_request)
                        .unwrap_or(0),
                    pids_limit: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.pids_limit)
                        .unwrap_or(0),
                    oom_score_adj: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.oom_score_adj)
                        .unwrap_or(0),
                });
                i += 2;
            }
            "--pids-limit" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --pids-limit");
                    return Ok(());
                }
                let pids_limit: u32 = args[i + 1]
                    .parse()
                    .map_err(|_| format!("Invalid pids limit: {}", args[i + 1]))?;
                request.resource_limits = Some(process_manager::ResourceLimits {
                    pids_limit,
                    cpu_limit: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.cpu_limit)
                        .unwrap_or(0),
                    cpu_request: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.cpu_request)
                        .unwrap_or(0),
                    memory_limit: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.memory_limit)
                        .unwrap_or(0),
                    memory_request: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.memory_request)
                        .unwrap_or(0),
                    oom_score_adj: request
                        .resource_limits
                        .as_ref()
                        .map(|r| r.oom_score_adj)
                        .unwrap_or(0),
                });
                i += 2;
            }
            "--env" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --env");
                    return Ok(());
                }
                request.env.push(args[i + 1].clone());
                i += 2;
            }
            "--env-file" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --env-file");
                    return Ok(());
                }
                request.env_file = Some(args[i + 1].clone());
                i += 2;
            }
            "--working-dir" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --working-dir");
                    return Ok(());
                }
                request.working_dir = Some(args[i + 1].clone());
                i += 2;
            }
            "--user" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --user");
                    return Ok(());
                }
                request.user = Some(args[i + 1].clone());
                i += 2;
            }
            "--group" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --group");
                    return Ok(());
                }
                request.group = Some(args[i + 1].clone());
                i += 2;
            }
            "--runtime-directory" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --runtime-directory");
                    return Ok(());
                }
                request.runtime_directory.push(args[i + 1].clone());
                i += 2;
            }
            "--ambient-capability" => {
                if i + 1 >= args.len() {
                    eprintln!("Missing value for --ambient-capability");
                    return Ok(());
                }
                request.ambient_capabilities.push(args[i + 1].clone());
                i += 2;
            }
            "--restart-process" => {
                request.restart_process = true;
                i += 1;
            }
            "--dry-run" => {
                request.dry_run = true;
                i += 1;
            }
            _ => {
                eprintln!("Unknown option: {}", arg);
                return Ok(());
            }
        }
    }

    // Track dry_run state before sending request
    let is_dry_run = request.dry_run;

    // Send update request
    let response = client.update(request).await?;
    let result = response.into_inner();

    if is_dry_run {
        println!("[OK] Dry run - validation successful (no changes applied)");
    } else {
        println!("[OK] Process configuration updated");
    }

    if !result.updated_fields.is_empty() {
        println!("\nUpdated fields:");
        for field in &result.updated_fields {
            println!("  - {}", field);
        }
    }
    if !result.restart_required_fields.is_empty() {
        println!("\nThe following fields require restart to take effect:");
        for field in &result.restart_required_fields {
            println!("  - {}", field);
        }
        if !result.process_restarted {
            println!(
                "\nUse --restart-process flag to restart the process and apply these changes."
            );
        }
    }
    if result.process_restarted {
        println!("\n✓ Process restarted successfully");
    }
    Ok(())
}

/// Handle health check command
pub async fn handle_health(
    client: &mut ProcessManagerClient<Channel>,
    args: &[String],
) -> Result<(), Box<dyn std::error::Error>> {
    // Parse flags
    let mut wait = false;
    let mut timeout_secs = 30u64;

    let mut i = 2;
    while i < args.len() {
        match args[i].as_str() {
            "--wait" | "-w" => {
                wait = true;
                i += 1;
            }
            "--timeout" | "-t" => {
                if i + 1 < args.len() {
                    timeout_secs = args[i + 1].parse().unwrap_or(30);
                    i += 2;
                } else {
                    return Err("--timeout requires a value".into());
                }
            }
            _ => {
                return Err(format!("Unknown flag: {}", args[i]).into());
            }
        }
    }

    if wait {
        // Poll health check until ready or timeout
        let start = std::time::Instant::now();
        let timeout = std::time::Duration::from_secs(timeout_secs);

        loop {
            match check_health(client).await {
                Ok(true) => {
                    println!("Daemon is healthy");
                    return Ok(());
                }
                Ok(false) => {
                    if start.elapsed() >= timeout {
                        eprintln!("Timeout waiting for daemon to become healthy");
                        std::process::exit(1);
                    }
                    // Wait a bit before retrying
                    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
                }
                Err(e) => {
                    if start.elapsed() >= timeout {
                        eprintln!("Timeout: {}", e);
                        std::process::exit(1);
                    }
                    // Connection error, retry
                    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
                }
            }
        }
    } else {
        // Single health check
        match check_health(client).await {
            Ok(true) => {
                println!("Daemon is healthy");
                Ok(())
            }
            Ok(false) => {
                eprintln!("Daemon is not healthy");
                std::process::exit(1);
            }
            Err(e) => {
                eprintln!("Health check failed: {}", e);
                std::process::exit(1);
            }
        }
    }
}

/// Check if daemon is healthy using standard gRPC health protocol
async fn check_health(
    _client: &mut ProcessManagerClient<Channel>,
) -> Result<bool, Box<dyn std::error::Error>> {
    // Use tonic-health client to check standard health endpoint
    // For now, we'll use GetStatus as a proxy for health
    // TODO: Use proper grpc.health.v1.Health/Check endpoint

    let request = tonic::Request::new(process_manager::GetStatusRequest {});

    match _client.get_status(request).await {
        Ok(response) => {
            let status = response.into_inner();
            Ok(status.ready)
        }
        Err(_) => Ok(false),
    }
}

/// Handle status command
pub async fn handle_status(
    client: &mut ProcessManagerClient<Channel>,
) -> Result<(), Box<dyn std::error::Error>> {
    let request = tonic::Request::new(process_manager::GetStatusRequest {});

    let response = client.get_status(request).await?;
    let status = response.into_inner();

    // Print status in a nice format
    println!("Daemon Status");
    println!("─────────────────────────────────────────");
    println!(
        "Ready:              {}",
        if status.ready { "✓" } else { "✗" }
    );
    println!("Version:            {}", status.version);
    println!("Uptime:             {} seconds", status.uptime_seconds);
    println!();
    println!("Process Statistics");
    println!("─────────────────────────────────────────");
    println!("Total:              {}", status.total_processes);
    println!("Running:            {}", status.running_processes);
    println!("Stopped:            {}", status.stopped_processes);
    println!("Failed:             {}", status.failed_processes);
    println!();
    println!("Health");
    println!("─────────────────────────────────────────");
    println!(
        "Supervisor:         {}",
        if status.supervisor_healthy {
            "✓"
        } else {
            "✗"
        }
    );
    println!(
        "Repository:         {}",
        if status.repository_healthy {
            "✓"
        } else {
            "✗"
        }
    );

    if !status.config_path.is_empty() {
        println!();
        println!("Configuration");
        println!("─────────────────────────────────────────");
        println!("Config Path:        {}", status.config_path);
    }

    Ok(())
}
