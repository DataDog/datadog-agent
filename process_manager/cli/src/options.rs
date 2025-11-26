//! Command-line options parsing for process creation

use std::collections::HashMap;

/// Macro to reduce repetitive flag parsing
macro_rules! parse_flag {
    // String option
    ($opts:expr, $args:expr, $i:expr, $flag:expr, $field:ident) => {{
        $i += 1;
        if $i >= $args.len() {
            return Err(format!("{} requires a value", $flag));
        }
        $opts.$field = Some($args[$i].clone());
    }};

    // Parsed option (u32, u64, i32, etc.)
    ($opts:expr, $args:expr, $i:expr, $flag:expr, $field:ident, parse) => {{
        $i += 1;
        if $i >= $args.len() {
            return Err(format!("{} requires a value", $flag));
        }
        $opts.$field = Some(
            $args[$i]
                .parse()
                .map_err(|_| format!("invalid {} value: {}", $flag, $args[$i]))?,
        );
    }};

    // Vec<String> (push)
    ($opts:expr, $args:expr, $i:expr, $flag:expr, $field:ident, vec) => {{
        $i += 1;
        if $i >= $args.len() {
            return Err(format!("{} requires a value", $flag));
        }
        $opts.$field.push($args[$i].clone());
    }};

    // Comma-separated list parsed into Vec<T>
    ($opts:expr, $args:expr, $i:expr, $flag:expr, $field:ident, csv) => {{
        $i += 1;
        if $i >= $args.len() {
            return Err(format!("{} requires a value", $flag));
        }
        let values: Result<Vec<_>, _> = $args[$i].split(',').map(|s| s.trim().parse()).collect();
        $opts.$field = Some(values.map_err(|_| format!("invalid {} value: {}", $flag, $args[$i]))?);
    }};

    // KEY=VALUE into HashMap
    ($opts:expr, $args:expr, $i:expr, $flag:expr, $field:ident, keyval) => {{
        $i += 1;
        if $i >= $args.len() {
            return Err(format!("{} requires a value", $flag));
        }
        let parts: Vec<&str> = $args[$i].splitn(2, '=').collect();
        if parts.len() != 2 {
            return Err(format!(
                "invalid {} format (use KEY=VALUE): {}",
                $flag, $args[$i]
            ));
        }
        $opts
            .$field
            .insert(parts[0].to_string(), parts[1].to_string());
    }};
}

/// Parse CPU value with unit suffix (e.g., "1000m" = 1000 millicores)
pub fn parse_cpu(value: &str) -> Result<u64, String> {
    if let Some(num_str) = value.strip_suffix('m') {
        // Millicores
        num_str
            .parse()
            .map_err(|_| format!("invalid CPU value: {}", value))
    } else {
        // Assume whole cores, convert to millicores
        value
            .parse::<u64>()
            .map(|n| n * 1000)
            .map_err(|_| format!("invalid CPU value: {}", value))
    }
}

/// Parse memory value with unit suffix (e.g., "256M", "1G")
pub fn parse_memory(value: &str) -> Result<u64, String> {
    let value_upper = value.to_uppercase();

    if let Some(num_str) = value_upper.strip_suffix("GB") {
        let num: u64 = num_str
            .parse()
            .map_err(|_| format!("invalid memory value: {}", value))?;
        Ok(num * 1024 * 1024 * 1024)
    } else if let Some(num_str) = value_upper.strip_suffix('G') {
        let num: u64 = num_str
            .parse()
            .map_err(|_| format!("invalid memory value: {}", value))?;
        Ok(num * 1024 * 1024 * 1024)
    } else if let Some(num_str) = value_upper.strip_suffix("MB") {
        let num: u64 = num_str
            .parse()
            .map_err(|_| format!("invalid memory value: {}", value))?;
        Ok(num * 1024 * 1024)
    } else if let Some(num_str) = value_upper.strip_suffix('M') {
        let num: u64 = num_str
            .parse()
            .map_err(|_| format!("invalid memory value: {}", value))?;
        Ok(num * 1024 * 1024)
    } else if let Some(num_str) = value_upper.strip_suffix("KB") {
        let num: u64 = num_str
            .parse()
            .map_err(|_| format!("invalid memory value: {}", value))?;
        Ok(num * 1024)
    } else if let Some(num_str) = value_upper.strip_suffix('K') {
        let num: u64 = num_str
            .parse()
            .map_err(|_| format!("invalid memory value: {}", value))?;
        Ok(num * 1024)
    } else {
        // Assume bytes
        value
            .parse()
            .map_err(|_| format!("invalid memory value: {}", value))
    }
}

/// Options for creating a new process
#[derive(Default)]
pub struct CreateOptions {
    pub name: String,
    pub command: String,
    pub args: Vec<String>,
    pub description: Option<String>,
    pub restart: Option<String>,
    pub restart_sec: Option<u64>,
    pub restart_max_delay: Option<u64>,
    pub start_limit_burst: Option<u32>,
    pub start_limit_interval: Option<u64>,
    pub working_dir: Option<String>,
    pub env_vars: HashMap<String, String>,
    pub environment_file: Option<String>,
    pub pidfile: Option<String>,
    pub stdout: Option<String>,
    pub stderr: Option<String>,
    pub timeout_start_sec: Option<u64>,
    pub timeout_stop_sec: Option<u64>,
    pub kill_signal: Option<String>,
    pub kill_mode: Option<String>,
    pub success_exit_status: Option<Vec<i32>>,
    pub exec_start_pre: Vec<String>,
    pub exec_start_post: Vec<String>,
    pub exec_stop_post: Vec<String>,
    pub user: Option<String>,
    pub group: Option<String>,
    pub auto_start: bool,
    // Dependencies
    pub after: Vec<String>,
    pub before: Vec<String>,
    pub requires: Vec<String>,
    pub wants: Vec<String>,
    pub binds_to: Vec<String>,
    pub conflicts: Vec<String>,
    // Process type
    pub process_type: Option<String>,
    // Health check
    pub health_check_type: Option<String>,
    pub health_check_interval: Option<u64>,
    pub health_check_timeout: Option<u64>,
    pub health_check_retries: Option<u32>,
    pub health_check_start_period: Option<u64>,
    // HTTP health check
    pub health_check_http_endpoint: Option<String>,
    pub health_check_http_method: Option<String>,
    pub health_check_http_status: Option<u16>,
    // TCP health check
    pub health_check_tcp_host: Option<String>,
    pub health_check_tcp_port: Option<u16>,
    // Exec health check
    pub health_check_exec_command: Option<String>,
    pub health_check_exec_args: Vec<String>,
    // Resource limits
    pub cpu_request: Option<String>,
    pub cpu_limit: Option<String>,
    pub memory_request: Option<String>,
    pub memory_limit: Option<String>,
    pub pids_limit: Option<u32>,
    pub oom_score_adj: Option<i32>,
    // Conditional execution
    pub condition_path_exists: Vec<String>,
    // Runtime directories
    pub runtime_directory: Vec<String>,
    // Ambient capabilities (Linux-only)
    pub ambient_capabilities: Vec<String>,
}

impl CreateOptions {
    /// Parse restart/lifecycle flags
    fn parse_restart_flags(
        &mut self,
        flag: &str,
        args: &[String],
        i: &mut usize,
    ) -> Result<bool, String> {
        match flag {
            "--restart" => parse_flag!(self, args, *i, "--restart", restart),
            "--restart-sec" => parse_flag!(self, args, *i, "--restart-sec", restart_sec, parse),
            "--restart-max-delay" => parse_flag!(
                self,
                args,
                *i,
                "--restart-max-delay",
                restart_max_delay,
                parse
            ),
            "--start-limit-burst" => parse_flag!(
                self,
                args,
                *i,
                "--start-limit-burst",
                start_limit_burst,
                parse
            ),
            "--start-limit-interval" => parse_flag!(
                self,
                args,
                *i,
                "--start-limit-interval",
                start_limit_interval,
                parse
            ),
            "--timeout-start" => {
                parse_flag!(self, args, *i, "--timeout-start", timeout_start_sec, parse)
            }
            "--timeout-stop" => {
                parse_flag!(self, args, *i, "--timeout-stop", timeout_stop_sec, parse)
            }
            "--success-exit-status" => parse_flag!(
                self,
                args,
                *i,
                "--success-exit-status",
                success_exit_status,
                csv
            ),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Parse execution environment flags
    fn parse_environment_flags(
        &mut self,
        flag: &str,
        args: &[String],
        i: &mut usize,
    ) -> Result<bool, String> {
        match flag {
            "--description" => parse_flag!(self, args, *i, "--description", description),
            "--working-dir" => parse_flag!(self, args, *i, "--working-dir", working_dir),
            "--environment-file" => {
                parse_flag!(self, args, *i, "--environment-file", environment_file)
            }
            "--pidfile" => parse_flag!(self, args, *i, "--pidfile", pidfile),
            "--stdout" => parse_flag!(self, args, *i, "--stdout", stdout),
            "--stderr" => parse_flag!(self, args, *i, "--stderr", stderr),
            "--user" => parse_flag!(self, args, *i, "--user", user),
            "--group" => parse_flag!(self, args, *i, "--group", group),
            "--auto-start" => {
                self.auto_start = true;
            }
            "--env" => parse_flag!(self, args, *i, "--env", env_vars, keyval),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Parse execution hooks and signals
    fn parse_execution_flags(
        &mut self,
        flag: &str,
        args: &[String],
        i: &mut usize,
    ) -> Result<bool, String> {
        match flag {
            "--exec-start-pre" => {
                parse_flag!(self, args, *i, "--exec-start-pre", exec_start_pre, vec)
            }
            "--exec-start-post" => {
                parse_flag!(self, args, *i, "--exec-start-post", exec_start_post, vec)
            }
            "--exec-stop-post" => {
                parse_flag!(self, args, *i, "--exec-stop-post", exec_stop_post, vec)
            }
            "--kill-signal" => parse_flag!(self, args, *i, "--kill-signal", kill_signal),
            "--kill-mode" => parse_flag!(self, args, *i, "--kill-mode", kill_mode),
            "--process-type" => parse_flag!(self, args, *i, "--process-type", process_type),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Parse dependency flags
    fn parse_dependency_flags(
        &mut self,
        flag: &str,
        args: &[String],
        i: &mut usize,
    ) -> Result<bool, String> {
        match flag {
            "--after" => parse_flag!(self, args, *i, "--after", after, vec),
            "--before" => parse_flag!(self, args, *i, "--before", before, vec),
            "--requires" => parse_flag!(self, args, *i, "--requires", requires, vec),
            "--wants" => parse_flag!(self, args, *i, "--wants", wants, vec),
            "--binds-to" => parse_flag!(self, args, *i, "--binds-to", binds_to, vec),
            "--conflicts" => parse_flag!(self, args, *i, "--conflicts", conflicts, vec),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Parse health check flags
    fn parse_health_check_flags(
        &mut self,
        flag: &str,
        args: &[String],
        i: &mut usize,
    ) -> Result<bool, String> {
        match flag {
            "--health-check-type" => {
                parse_flag!(self, args, *i, "--health-check-type", health_check_type)
            }
            "--health-check-interval" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-interval",
                health_check_interval,
                parse
            ),
            "--health-check-timeout" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-timeout",
                health_check_timeout,
                parse
            ),
            "--health-check-retries" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-retries",
                health_check_retries,
                parse
            ),
            "--health-check-start-period" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-start-period",
                health_check_start_period,
                parse
            ),
            "--health-check-http-endpoint" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-http-endpoint",
                health_check_http_endpoint
            ),
            "--health-check-http-method" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-http-method",
                health_check_http_method
            ),
            "--health-check-http-status" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-http-status",
                health_check_http_status,
                parse
            ),
            "--health-check-tcp-host" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-tcp-host",
                health_check_tcp_host
            ),
            "--health-check-tcp-port" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-tcp-port",
                health_check_tcp_port,
                parse
            ),
            "--health-check-exec-command" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-exec-command",
                health_check_exec_command
            ),
            "--health-check-exec-arg" => parse_flag!(
                self,
                args,
                *i,
                "--health-check-exec-arg",
                health_check_exec_args,
                vec
            ),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Parse resource limit flags
    fn parse_resource_flags(
        &mut self,
        flag: &str,
        args: &[String],
        i: &mut usize,
    ) -> Result<bool, String> {
        match flag {
            "--cpu-request" => parse_flag!(self, args, *i, "--cpu-request", cpu_request),
            "--cpu-limit" => parse_flag!(self, args, *i, "--cpu-limit", cpu_limit),
            "--memory-request" => parse_flag!(self, args, *i, "--memory-request", memory_request),
            "--memory-limit" => parse_flag!(self, args, *i, "--memory-limit", memory_limit),
            "--pids-limit" => parse_flag!(self, args, *i, "--pids-limit", pids_limit, parse),
            "--oom-score-adj" => {
                parse_flag!(self, args, *i, "--oom-score-adj", oom_score_adj, parse)
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Parse conditional and security flags
    fn parse_advanced_flags(
        &mut self,
        flag: &str,
        args: &[String],
        i: &mut usize,
    ) -> Result<bool, String> {
        match flag {
            "--condition-path-exists" => parse_flag!(
                self,
                args,
                *i,
                "--condition-path-exists",
                condition_path_exists,
                vec
            ),
            "--runtime-directory" => parse_flag!(
                self,
                args,
                *i,
                "--runtime-directory",
                runtime_directory,
                vec
            ),
            "--ambient-capability" => parse_flag!(
                self,
                args,
                *i,
                "--ambient-capability",
                ambient_capabilities,
                vec
            ),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// Parse command-line arguments into CreateOptions
    ///
    /// Expected format: create <name> <command> [args...] [FLAGS]
    pub fn parse(args: &[String]) -> Result<Self, String> {
        if args.len() < 4 {
            return Err("insufficient arguments".to_string());
        }

        let mut opts = CreateOptions {
            name: args[2].clone(),
            command: args[3].clone(),
            ..Default::default()
        };

        let mut i = 4;
        while i < args.len() {
            let arg = &args[i];

            // Check if this is a flag
            if arg.starts_with("--") {
                // Try each category of flags
                if opts.parse_restart_flags(arg, args, &mut i)?
                    || opts.parse_environment_flags(arg, args, &mut i)?
                    || opts.parse_execution_flags(arg, args, &mut i)?
                    || opts.parse_dependency_flags(arg, args, &mut i)?
                    || opts.parse_health_check_flags(arg, args, &mut i)?
                    || opts.parse_resource_flags(arg, args, &mut i)?
                    || opts.parse_advanced_flags(arg, args, &mut i)?
                {
                    // Flag was handled
                } else {
                    return Err(format!("unknown flag: {}", arg));
                }
            } else {
                // Not a flag, must be a command argument
                opts.args.push(arg.clone());
            }

            i += 1;
        }

        Ok(opts)
    }

    /// Print usage information for the create command
    pub fn print_usage() {
        eprintln!("usage: cli create <name> <command> [args...] [OPTIONS]");
        eprintln!();
        eprintln!("Arguments:");
        eprintln!("  <name>       Unique name for the process");
        eprintln!("  <command>    Command to execute");
        eprintln!("  [args...]    Optional arguments for the command");
        eprintln!();
        eprintln!("Options:");
        eprintln!("  --description <text>            Human-readable description");
        eprintln!("  --restart <policy>              Restart policy: never, always, on-failure, on-success");
        eprintln!("  --restart-sec <seconds>         Base restart delay (default: 1)");
        eprintln!("  --restart-max-delay <seconds>   Maximum restart delay cap (default: 60)");
        eprintln!("  --start-limit-burst <count>     Max restart attempts (default: 5)");
        eprintln!("  --start-limit-interval <secs>   Time window for start limits (default: 300)");
        eprintln!("  --working-dir <path>            Working directory");
        eprintln!("  --env KEY=VALUE                 Environment variable (can be repeated)");
        eprintln!("  --environment-file <path>       Load environment variables from file (KEY=VALUE format, prefix with '-' to ignore if missing)");
        eprintln!("  --pidfile <path>                Write PID to file on start, remove on stop");
        eprintln!("  --stdout <file|inherit|null>    Redirect stdout to file, inherit, or null");
        eprintln!("  --stderr <file|inherit|null>    Redirect stderr to file, inherit, or null");
        eprintln!("  --timeout-start <seconds>       Max time to wait for process to start (0 = no timeout)");
        eprintln!("  --timeout-stop <seconds>        Max time to wait for graceful stop before SIGKILL (default: 90)");
        eprintln!("  --kill-signal <signal>          Signal to send on stop: SIGTERM, SIGINT, SIGKILL, etc. (default: SIGTERM)");
        eprintln!("  --kill-mode <mode>              How to kill child processes: control-group, process-group, process, mixed (default: control-group)");
        eprintln!("  --success-exit-status <codes>   Comma-separated list of exit codes considered success (default: 0)");
        eprintln!(
            "  --exec-start-pre <command>      Command to run before starting (can be repeated)"
        );
        eprintln!(
            "  --exec-start-post <command>     Command to run after starting (can be repeated)"
        );
        eprintln!(
            "  --exec-stop-post <command>      Command to run after stopping (can be repeated)"
        );
        eprintln!("  --user <username or UID>        User to run the process as");
        eprintln!("  --group <groupname or GID>      Group to run the process as");
        eprintln!("  --auto-start                    Start immediately after creation");
        eprintln!();
        eprintln!("Dependencies (systemd-like):");
        eprintln!("  --after <name>                  Start after this process (can be repeated)");
        eprintln!("  --before <name>                 Start before this process (can be repeated)");
        eprintln!(
            "  --requires <name>               Hard dependency - must be running (can be repeated)"
        );
        eprintln!(
            "  --wants <name>                  Soft dependency - warn if missing (can be repeated)"
        );
        eprintln!(
            "  --binds-to <name>               Strong binding - if target stops, this stops (can be repeated)"
        );
        eprintln!(
            "  --conflicts <name>              Mutual exclusion - cannot run if specified process is running (can be repeated)"
        );
        eprintln!();
        eprintln!("Process Types:");
        eprintln!("  --process-type <type>           Process type: simple, forking, oneshot, notify (default: simple)");
        eprintln!();
        eprintln!("Health Checks:");
        eprintln!("  --health-check-type <type>      Health check type: http, tcp, exec");
        eprintln!("  --health-check-interval <secs>  Seconds between health checks (default: 30)");
        eprintln!("  --health-check-timeout <secs>   Health check timeout in seconds (default: 5)");
        eprintln!("  --health-check-retries <count>  Failed checks before restart (default: 3)");
        eprintln!("  --health-check-start-period <s> Grace period before first check (default: 0)");
        eprintln!();
        eprintln!("  HTTP Health Check:");
        eprintln!("    --health-check-http-endpoint <url>     HTTP endpoint to check");
        eprintln!("    --health-check-http-method <method>    HTTP method (default: GET)");
        eprintln!("    --health-check-http-status <code>      Expected status code (default: 200)");
        eprintln!();
        eprintln!("  TCP Health Check:");
        eprintln!("    --health-check-tcp-host <host>         TCP host to check");
        eprintln!("    --health-check-tcp-port <port>         TCP port to check");
        eprintln!();
        eprintln!("  Exec Health Check:");
        eprintln!("    --health-check-exec-command <cmd>      Command to execute");
        eprintln!("    --health-check-exec-arg <arg>          Command argument (can be repeated)");
        eprintln!();
        eprintln!("Resource Limits (cgroup v2):");
        eprintln!("  --cpu-request <millicores>      Minimum CPU (e.g., 100m, 1000m = 1 core)");
        eprintln!("  --cpu-limit <millicores>        Maximum CPU (e.g., 2000m = 2 cores)");
        eprintln!("  --memory-request <size>         Minimum memory (e.g., 128M, 1G)");
        eprintln!("  --memory-limit <size>           Maximum memory (e.g., 512M, 2G)");
        eprintln!("  --pids-limit <count>            Maximum number of PIDs/threads");
        eprintln!("  --oom-score-adj <score>         OOM killer score (-1000 to 1000)");
        eprintln!();
        eprintln!("Conditional Execution:");
        eprintln!(
            "  --condition-path-exists <path>  Check path exists before starting (repeatable)"
        );
        eprintln!("                                  Prefix with ! for 'must NOT exist'");
        eprintln!("                                  Prefix with | for 'OR logic' (at least one)");
        eprintln!(
            "  --runtime-directory <name>      Create directory under /run/ on start (repeatable)"
        );
        eprintln!("                                  Directories are removed on stop");
        eprintln!(
            "  --ambient-capability <cap>      Grant Linux capability without root (repeatable)"
        );
        eprintln!("                                  Example: CAP_NET_BIND_SERVICE (Linux-only)");
        eprintln!();
        eprintln!("Examples:");
        eprintln!("  cli create webserver python3 -m http.server 8080");
        eprintln!("  cli create api ./server --restart always --auto-start");
        eprintln!("  cli create worker ./worker --restart on-failure --restart-sec 5 --env QUEUE_URL=redis://localhost");
        eprintln!("  cli create webapp gunicorn app:app --after database --requires database");
        eprintln!("  cli create web python3 -m http.server 8080 --health-check-type http --health-check-http-endpoint http://localhost:8080/");
    }
}
