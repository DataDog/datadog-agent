//! Tokio Process Executor
//! Real implementation of ProcessExecutor port using tokio

use crate::domain::{
    ports::{ProcessExecutor, SpawnConfig, SpawnResult},
    use_cases::ResourceUsageReader,
    DomainError, ResourceUsage,
};
use async_trait::async_trait;
use std::collections::HashMap;
use std::fs::{self, File, OpenOptions};
use std::io::{BufRead, BufReader};
use std::os::unix::process::CommandExt; // For pre_exec
use std::path::Path;
use std::process::{Command, Stdio};
use tracing::{debug, error, info, warn};

/// Tokio-based process executor
///
/// This adapter translates domain operations into actual system calls
pub struct TokioProcessExecutor {
    cgroup_available: bool,
}

impl TokioProcessExecutor {
    pub fn new() -> Self {
        let cgroup_available = Self::detect_cgroup_v2();

        if cgroup_available {
            info!("cgroup v2 detected and available for resource limits");
        } else {
            warn!("cgroup v2 not available, will use rlimit fallback for resource limits");
        }

        Self { cgroup_available }
    }

    /// Detect if cgroup v2 is available and usable
    /// This is checked once at initialization
    fn detect_cgroup_v2() -> bool {
        #[cfg(target_os = "linux")]
        {
            use std::fs;

            let cgroup_path = std::path::Path::new("/sys/fs/cgroup");

            // Check if cgroup v2 exists
            if !cgroup_path.exists() {
                return false;
            }

            // Check for cgroup.controllers (indicates cgroup v2)
            let controllers_file = cgroup_path.join("cgroup.controllers");
            if !controllers_file.exists() {
                return false;
            }

            // Try to read controllers to verify access and check for useful controllers
            if let Ok(controllers) = fs::read_to_string(&controllers_file) {
                let has_cpu = controllers.contains("cpu");
                let has_memory = controllers.contains("memory");
                let has_pids = controllers.contains("pids");

                // Need at least one useful controller
                return has_cpu || has_memory || has_pids;
            }

            false
        }

        #[cfg(not(target_os = "linux"))]
        {
            false
        }
    }
}

impl Default for TokioProcessExecutor {
    fn default() -> Self {
        Self::new()
    }
}

// Helper functions
impl TokioProcessExecutor {
    /// Load environment variables from a file
    /// Format: KEY=VALUE, one per line, # for comments
    ///
    /// Supports systemd-style optional prefix:
    /// - If path starts with '-', the file is optional (no error if missing)
    fn load_env_file(path: &str) -> Result<HashMap<String, String>, DomainError> {
        // Handle optional file prefix (systemd-style: -/path/to/file)
        let (actual_path, optional) = if let Some(stripped) = path.strip_prefix('-') {
            (stripped, true)
        } else {
            (path, false)
        };

        let file = match File::open(actual_path) {
            Ok(f) => f,
            Err(e) => {
                if optional {
                    // Optional file doesn't exist - return empty env vars
                    debug!(
                        file = actual_path,
                        "Optional environment file not found, continuing without it"
                    );
                    return Ok(HashMap::new());
                }
                return Err(DomainError::InvalidCommand(format!(
                    "Failed to open environment file '{}': {}",
                    actual_path, e
                )));
            }
        };

        let reader = BufReader::new(file);
        let mut env_vars = HashMap::new();

        for (line_num, line) in reader.lines().enumerate() {
            let line = line.map_err(|e| {
                DomainError::InvalidCommand(format!(
                    "Failed to read environment file '{}': {}",
                    actual_path, e
                ))
            })?;

            let line = line.trim();

            // Skip empty lines and comments
            if line.is_empty() || line.starts_with('#') {
                continue;
            }

            // Parse KEY=VALUE
            if let Some((key, value)) = line.split_once('=') {
                env_vars.insert(key.trim().to_string(), value.trim().to_string());
            } else {
                warn!(
                    file = actual_path,
                    line = line_num + 1,
                    content = line,
                    "Invalid line in environment file (expected KEY=VALUE format)"
                );
            }
        }

        debug!(
            file = actual_path,
            count = env_vars.len(),
            "Loaded environment variables from file"
        );
        Ok(env_vars)
    }

    /// Configure stdout redirection
    fn configure_stdout(stdout_config: Option<&str>) -> Result<Stdio, DomainError> {
        match stdout_config {
            None | Some("null") => Ok(Stdio::null()),
            Some("inherit") => Ok(Stdio::inherit()),
            Some(path) => {
                let file = OpenOptions::new()
                    .create(true)
                    .append(true)
                    .open(path)
                    .map_err(|e| {
                        DomainError::InvalidCommand(format!(
                            "Failed to open stdout file '{}': {}",
                            path, e
                        ))
                    })?;
                Ok(Stdio::from(file))
            }
        }
    }

    /// Configure stderr redirection
    fn configure_stderr(stderr_config: Option<&str>) -> Result<Stdio, DomainError> {
        match stderr_config {
            None | Some("null") => Ok(Stdio::null()),
            Some("inherit") => Ok(Stdio::inherit()),
            Some(path) => {
                let file = OpenOptions::new()
                    .create(true)
                    .append(true)
                    .open(path)
                    .map_err(|e| {
                        DomainError::InvalidCommand(format!(
                            "Failed to open stderr file '{}': {}",
                            path, e
                        ))
                    })?;
                Ok(Stdio::from(file))
            }
        }
    }

    /// Write PID to file
    fn write_pidfile(path: &str, pid: u32) -> Result<(), DomainError> {
        use std::io::Write;

        let mut file = OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .open(path)
            .map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to create PID file '{}': {}", path, e))
            })?;

        write!(file, "{}", pid).map_err(|e| {
            DomainError::InvalidCommand(format!("Failed to write to PID file '{}': {}", path, e))
        })?;

        debug!(pidfile = path, pid = pid, "Wrote PID file");
        Ok(())
    }

    /// Apply resource limits to a process using cgroups (Linux only)
    ///
    /// Note: This requires cgroups v2 and appropriate permissions.
    /// On systems without cgroups or without permissions, this will log a warning and continue.
    #[cfg(target_os = "linux")]
    fn apply_resource_limits(
        pid: u32,
        limits: &crate::domain::ResourceLimits,
    ) -> Result<(), DomainError> {
        debug!(
            pid = pid,
            limits = %limits,
            "Applying resource limits"
        );

        // Check if cgroups v2 is available
        let cgroup_root = std::path::Path::new("/sys/fs/cgroup");
        if !cgroup_root.exists() {
            debug!("cgroups not available, skipping resource limits");
            return Ok(());
        }

        // Create a cgroup for this process under pm/<pid>
        let cgroup_path = format!("/sys/fs/cgroup/pm-{}", pid);
        let cgroup_dir = std::path::Path::new(&cgroup_path);

        // Try to create the cgroup directory
        if let Err(e) = std::fs::create_dir_all(cgroup_dir) {
            warn!(
                pid = pid,
                error = %e,
                "Failed to create cgroup directory (insufficient permissions?)"
            );
            return Ok(()); // Continue without cgroups
        }

        // Add process to cgroup
        let procs_path = cgroup_dir.join("cgroup.procs");
        if let Err(e) = std::fs::write(&procs_path, format!("{}", pid)) {
            warn!(
                pid = pid,
                error = %e,
                "Failed to add process to cgroup"
            );
            // Clean up cgroup directory
            let _ = std::fs::remove_dir(cgroup_dir);
            return Ok(());
        }

        // Apply CPU limit
        if let Some(cpu_millis) = limits.cpu_millis {
            // Convert millicores to cgroup format: quota/period
            // 1 core = 1000 millicores = 100000 quota with 100000 period
            let period = 100_000;
            let quota = (cpu_millis * period) / 1000;

            let cpu_max_path = cgroup_dir.join("cpu.max");
            let cpu_max_value = format!("{} {}", quota, period);

            if let Err(e) = std::fs::write(&cpu_max_path, &cpu_max_value) {
                warn!(
                    pid = pid,
                    error = %e,
                    "Failed to set CPU limit"
                );
            } else {
                debug!(pid = pid, cpu_millis = cpu_millis, "Applied CPU limit");
            }
        }

        // Apply memory limit
        if let Some(memory_bytes) = limits.memory_bytes {
            let memory_max_path = cgroup_dir.join("memory.max");
            if let Err(e) = std::fs::write(&memory_max_path, format!("{}", memory_bytes)) {
                warn!(
                    pid = pid,
                    error = %e,
                    "Failed to set memory limit"
                );
            } else {
                debug!(
                    pid = pid,
                    memory_bytes = memory_bytes,
                    "Applied memory limit"
                );
            }
        }

        // Apply PIDs limit
        if let Some(max_pids) = limits.max_pids {
            let pids_max_path = cgroup_dir.join("pids.max");
            if let Err(e) = std::fs::write(&pids_max_path, format!("{}", max_pids)) {
                warn!(
                    pid = pid,
                    error = %e,
                    "Failed to set PIDs limit"
                );
            } else {
                debug!(pid = pid, max_pids = max_pids, "Applied PIDs limit");
            }
        }

        Ok(())
    }

    /// Stub for non-Linux platforms
    #[cfg(not(target_os = "linux"))]
    fn apply_resource_limits(
        _pid: u32,
        _limits: &crate::domain::ResourceLimits,
    ) -> Result<(), DomainError> {
        debug!("Resource limits not supported on this platform");
        Ok(())
    }

    /// Apply rlimits from within child process (for pre_exec)
    /// Must be called from within the child process before exec
    /// This is a fallback when cgroups are not available
    #[cfg(unix)]
    fn apply_rlimits_in_child(limits: &crate::domain::ResourceLimits) -> std::io::Result<()> {
        use libc::{rlimit, setrlimit, RLIMIT_AS, RLIMIT_CPU};

        // Set memory limit (address space)
        if let Some(memory_bytes) = limits.memory_bytes {
            let limit = rlimit {
                rlim_cur: memory_bytes,
                rlim_max: memory_bytes,
            };
            unsafe {
                if setrlimit(RLIMIT_AS, &limit) != 0 {
                    return Err(std::io::Error::last_os_error());
                }
            }
        }

        // Set CPU limit (seconds of CPU time)
        // Note: This is total CPU time, not throttling like cgroups
        // Rough approximation: assume process runs for 1 hour
        // 1000 millicores = 3600 seconds, 500 millicores = 1800 seconds
        if let Some(cpu_millicores) = limits.cpu_millis {
            let cpu_seconds = (cpu_millicores * 3600) / 1000;
            let limit = rlimit {
                rlim_cur: cpu_seconds,
                rlim_max: cpu_seconds,
            };
            unsafe {
                if setrlimit(RLIMIT_CPU, &limit) != 0 {
                    return Err(std::io::Error::last_os_error());
                }
            }
        }

        // Note: RLIMIT_NPROC limits number of processes for the user, not this specific process
        // Not setting it to avoid affecting other processes

        Ok(())
    }

    /// Stub for non-Unix platforms
    #[cfg(not(unix))]
    fn apply_rlimits_in_child(_limits: &crate::domain::ResourceLimits) -> std::io::Result<()> {
        Ok(())
    }

    /// Resolve username to UID
    #[cfg(unix)]
    fn get_uid(user: &str) -> Result<u32, DomainError> {
        use std::ffi::CString;

        let user_cstr = CString::new(user).map_err(|e| {
            DomainError::InvalidCommand(format!("Invalid user string '{}': {}", user, e))
        })?;

        unsafe {
            let pwd = libc::getpwnam(user_cstr.as_ptr());
            if pwd.is_null() {
                return Err(DomainError::InvalidCommand(format!(
                    "User '{}' not found",
                    user
                )));
            }
            Ok((*pwd).pw_uid)
        }
    }

    /// Resolve group name to GID
    #[cfg(unix)]
    fn get_gid(group: &str) -> Result<u32, DomainError> {
        use std::ffi::CString;

        let group_cstr = CString::new(group).map_err(|e| {
            DomainError::InvalidCommand(format!("Invalid group string '{}': {}", group, e))
        })?;

        unsafe {
            let grp = libc::getgrnam(group_cstr.as_ptr());
            if grp.is_null() {
                return Err(DomainError::InvalidCommand(format!(
                    "Group '{}' not found",
                    group
                )));
            }
            Ok((*grp).gr_gid)
        }
    }

    /// Apply ambient capabilities (Linux-only, must be called before setuid)
    ///
    /// Ambient capabilities are preserved across execve() and setuid(), allowing
    /// unprivileged processes to retain specific capabilities.
    ///
    /// To set an ambient capability, it must first be in BOTH the permitted AND
    /// inheritable sets. We use caps::raise() to add individual capabilities
    /// rather than caps::set() which replaces the entire set.
    #[cfg(target_os = "linux")]
    fn apply_ambient_capabilities(capabilities: &[String]) -> std::io::Result<()> {
        use caps::{CapSet, Capability};

        for cap_str in capabilities {
            // Parse capability string (e.g., "CAP_NET_BIND_SERVICE")
            let capability = cap_str.parse::<Capability>().map_err(|e| {
                std::io::Error::new(
                    std::io::ErrorKind::InvalidInput,
                    format!("Invalid capability '{}': {}", cap_str, e),
                )
            })?;

            // To set ambient capabilities, we need to:
            // 1. Ensure capability is in permitted set (raise, not replace)
            // 2. Ensure capability is in inheritable set (raise, not replace)
            // 3. Raise the ambient capability
            //
            // Using raise() adds to existing set; set() would replace entirely
            caps::raise(None, CapSet::Permitted, capability).map_err(|e| {
                std::io::Error::new(
                    std::io::ErrorKind::PermissionDenied,
                    format!("Failed to raise {} in permitted set: {}", cap_str, e),
                )
            })?;

            caps::raise(None, CapSet::Inheritable, capability).map_err(|e| {
                std::io::Error::new(
                    std::io::ErrorKind::PermissionDenied,
                    format!("Failed to raise {} in inheritable set: {}", cap_str, e),
                )
            })?;

            caps::raise(None, CapSet::Ambient, capability).map_err(|e| {
                std::io::Error::new(
                    std::io::ErrorKind::PermissionDenied,
                    format!("Failed to raise {} in ambient set: {}", cap_str, e),
                )
            })?;

            debug!(capability = %cap_str, "Set ambient capability");
        }

        Ok(())
    }

    /// Stub for non-Linux platforms (ambient capabilities only work on Linux)
    #[cfg(not(target_os = "linux"))]
    #[allow(dead_code)]
    fn apply_ambient_capabilities(_capabilities: &[String]) -> std::io::Result<()> {
        Ok(())
    }
}

#[async_trait]
impl ProcessExecutor for TokioProcessExecutor {
    async fn spawn(&self, config: SpawnConfig) -> Result<SpawnResult, DomainError> {
        info!(
            command = %config.command,
            args = ?config.args,
            "Spawning process"
        );

        // Validate command
        if config.command.is_empty() {
            return Err(DomainError::InvalidCommand("Empty command".to_string()));
        }

        // Build command with args
        let mut cmd = Command::new(&config.command);
        cmd.args(&config.args);

        // Set working directory
        if let Some(ref dir) = config.working_dir {
            debug!(working_dir = %dir, "Setting working directory");
            cmd.current_dir(dir);
        }

        // Load environment from file if specified
        let mut all_env_vars = HashMap::new();
        if let Some(ref env_file) = config.environment_file {
            let file_vars = Self::load_env_file(env_file)?;
            all_env_vars.extend(file_vars);
        }

        // Add explicit environment variables (override file vars if conflict)
        for (key, value) in &config.env_vars {
            all_env_vars.insert(key.clone(), value.clone());
        }

        // Apply all environment variables
        if !all_env_vars.is_empty() {
            debug!(count = all_env_vars.len(), "Setting environment variables");
            for (key, value) in &all_env_vars {
                cmd.env(key, value);
            }
        }

        // Socket activation: Set LISTEN_FDS and LISTEN_PID environment variables
        // This implements systemd socket activation protocol
        if !config.listen_fds.is_empty() {
            let num_fds = config.listen_fds.len();
            debug!(num_fds = num_fds, "Setting up socket activation");

            // Standard systemd socket activation protocol
            // LISTEN_FDS: Number of file descriptors being passed
            cmd.env("LISTEN_FDS", num_fds.to_string());

            // LISTEN_PID: Will be set to the child process PID after spawn
            // For now, we set it to "self" - it will be updated post-spawn
            // Note: The child should verify LISTEN_PID == getpid() for security
            cmd.env("LISTEN_PID", "0"); // Placeholder, will be set after spawn

            // DataDog-specific socket activation (non-standard)
            // DataDog trace-agent uses DD_APM_NET_RECEIVER_FD instead of LISTEN_FDS
            // It expects a comma-separated list of FD numbers starting at 3
            let fd_list: Vec<String> = (3..3 + num_fds).map(|n| n.to_string()).collect();
            cmd.env("DD_APM_NET_RECEIVER_FD", fd_list.join(","));

            info!(
                command = %config.command,
                num_fds = num_fds,
                dd_fds = %fd_list.join(","),
                "Socket activation: passing {} file descriptor(s)",
                num_fds
            );
        }

        // Configure stdio
        cmd.stdin(Stdio::null());
        cmd.stdout(Self::configure_stdout(config.stdout.as_deref())?);
        cmd.stderr(Self::configure_stderr(config.stderr.as_deref())?);

        // Resolve user/group to UID/GID upfront (before pre_exec)
        #[cfg(unix)]
        let (uid_opt, gid_opt) = {
            let uid = if let Some(ref user) = config.user {
                let uid = Self::get_uid(user)?;
                debug!(user = %user, uid = uid, "Resolved process user");
                Some(uid)
            } else {
                None
            };

            let gid = if let Some(ref group) = config.group {
                let gid = Self::get_gid(group)?;
                debug!(group = %group, gid = gid, "Resolved process group");
                Some(gid)
            } else {
                None
            };

            (uid, gid)
        };

        // Set up pre_exec for: setsid, rlimits, capabilities, user/group switching
        // Note: Multiple pre_exec() calls overwrite each other, so we must combine all logic!
        #[cfg(unix)]
        {
            let use_rlimit = !self.cgroup_available && config.resource_limits.has_limits();
            #[cfg(target_os = "linux")]
            let has_capabilities = !config.ambient_capabilities.is_empty();

            // Always use pre_exec to create a new session (for process group management)
            let limits = config.resource_limits.clone();
            #[cfg(target_os = "linux")]
            let capabilities = config.ambient_capabilities.clone();
            let socket_fds = config.listen_fds.clone(); // Socket FDs for socket activation
            let has_socket_fds = !socket_fds.is_empty();

            unsafe {
                cmd.pre_exec(move || {
                    // 0. Create new session/process group (for process group management)
                    // This ensures that kill_with_mode(ProcessGroup) works correctly
                    if libc::setsid() < 0 {
                        // Ignore error if already session leader, it's not critical
                    }

                    // 1. Duplicate socket FDs to FD 3, 4, 5, etc. (systemd socket activation)
                    // This MUST happen before setuid/setgid to ensure proper FD inheritance
                    if has_socket_fds {
                        for (i, &fd) in socket_fds.iter().enumerate() {
                            let target_fd = 3 + i as i32;
                            if libc::dup2(fd, target_fd) == -1 {
                                return Err(std::io::Error::last_os_error());
                            }
                        }
                    }

                    // 2. Apply rlimits (if cgroups unavailable)
                    if use_rlimit {
                        Self::apply_rlimits_in_child(&limits)?;
                    }

                    // 3. Set ambient capabilities (Linux-only)
                    // This is a multi-step process because setuid() clears ambient capabilities:
                    // a) Set SECBIT_KEEP_CAPS to preserve permitted set across setuid
                    // b) Raise capability in permitted and inheritable sets
                    // c) Do setuid (permitted preserved, but ambient cleared)
                    // d) Raise capability in ambient set again after setuid
                    #[cfg(target_os = "linux")]
                    let needs_caps_after_setuid = has_capabilities && uid_opt.is_some();

                    #[cfg(target_os = "linux")]
                    if has_capabilities {
                        // Set SECBIT_KEEP_CAPS to preserve capabilities across setuid
                        // This is required because setuid() from root to non-root clears capabilities
                        const PR_SET_SECUREBITS: libc::c_int = 28;
                        const SECBIT_KEEP_CAPS: libc::c_ulong = 0x10;
                        if libc::prctl(PR_SET_SECUREBITS, SECBIT_KEEP_CAPS) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }

                        // Raise capabilities in permitted and inheritable sets
                        Self::apply_ambient_capabilities(&capabilities)?;
                    }

                    // 4. Set group (must be before setuid drops privileges)
                    if let Some(gid) = gid_opt {
                        if libc::setgid(gid) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }
                        eprintln!("Set GID to {}", gid);
                    }

                    // 5. Set user (drops privileges)
                    if let Some(uid) = uid_opt {
                        if libc::setuid(uid) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }
                        eprintln!("Set UID to {}", uid);
                    }

                    // 6. Re-raise ambient capabilities after setuid
                    // setuid() clears the ambient set even with SECBIT_KEEP_CAPS,
                    // but permitted and inheritable are preserved, so we can raise ambient again
                    #[cfg(target_os = "linux")]
                    if needs_caps_after_setuid {
                        Self::apply_ambient_capabilities(&capabilities)?;
                    }

                    Ok(())
                });
            }
        }

        // Spawn the process
        let child = cmd.spawn().map_err(|e| {
            error!(
                command = %config.command,
                error = %e,
                "Failed to spawn process"
            );
            DomainError::InvalidCommand(format!("Failed to spawn process: {}", e))
        })?;

        let pid = child.id();

        // Write PID file if configured
        if let Some(ref pidfile) = config.pidfile {
            Self::write_pidfile(pidfile, pid)?;
        }

        // Apply cgroup resource limits if configured and cgroups are available
        // (If using rlimit fallback, limits were already applied via pre_exec)
        if self.cgroup_available && config.resource_limits.has_limits() {
            if let Err(e) = Self::apply_resource_limits(pid, &config.resource_limits) {
                warn!(
                    pid = pid,
                    error = %e,
                    "Failed to apply cgroup resource limits (continuing anyway)"
                );
            }
        }

        info!(pid = pid, "Process spawned successfully");

        // Create an exit handle that can be awaited for process termination
        // This enables event-driven monitoring without polling
        //
        // IMPORTANT: We spawn a background task to wait on the child so it gets
        // reaped properly. The task sends the exit code via a oneshot channel.
        // This prevents zombie processes while still providing event-driven monitoring.
        let exit_handle = {
            use crate::domain::ports::ProcessExitHandle;
            let mut child = child;
            let (tx, rx) = tokio::sync::oneshot::channel();

            // Spawn a background task to wait on the child
            // Use spawn_blocking since std::process::Child::wait() is blocking
            tokio::spawn(async move {
                let exit_result = tokio::task::spawn_blocking(move || match child.wait() {
                    Ok(status) => {
                        let exit_code = status.code().unwrap_or(1);
                        debug!(pid = pid, exit_code = exit_code, "Process exited");
                        Ok(exit_code)
                    }
                    Err(e) => {
                        error!(pid = pid, error = %e, "Failed to wait for process");
                        Err(DomainError::InvalidCommand(format!(
                            "Failed to wait for process: {}",
                            e
                        )))
                    }
                })
                .await
                .unwrap_or_else(|e| {
                    error!(error = %e, "Blocking task panicked");
                    Err(DomainError::InvalidCommand(format!(
                        "Wait task panicked: {}",
                        e
                    )))
                });
                // Send the result (ignore if receiver dropped)
                let _ = tx.send(exit_result);
            });

            // Return a future that waits for the exit code from the channel
            let exit_fut = async move {
                match rx.await {
                    Ok(result) => result,
                    Err(_) => {
                        // Channel closed without sending (shouldn't happen)
                        Err(DomainError::InvalidCommand(
                            "Process monitor task died unexpectedly".to_string(),
                        ))
                    }
                }
            };
            Some(Box::pin(exit_fut) as ProcessExitHandle)
        };

        Ok(SpawnResult { pid, exit_handle })
    }

    async fn kill(&self, pid: u32, signal: i32) -> Result<(), DomainError> {
        info!(pid = pid, signal = signal, "Killing process");

        #[cfg(unix)]
        {
            let result = unsafe { libc::kill(pid as i32, signal) };
            if result != 0 {
                let err = std::io::Error::last_os_error();
                warn!(
                    pid = pid,
                    signal = signal,
                    error = %err,
                    "Failed to send signal to process"
                );
                return Err(DomainError::InvalidCommand(format!(
                    "Failed to send signal {}: {}",
                    signal, err
                )));
            }

            debug!(pid = pid, signal = signal, "Signal sent successfully");
            Ok(())
        }

        #[cfg(not(unix))]
        {
            Err(DomainError::InvalidCommand(
                "Process killing not implemented on non-Unix platforms".to_string(),
            ))
        }
    }

    async fn kill_with_mode(
        &self,
        pid: u32,
        signal: i32,
        mode: crate::domain::KillMode,
    ) -> Result<(), DomainError> {
        use crate::domain::KillMode;

        info!(
            pid = pid,
            signal = signal,
            mode = %mode,
            "Killing process with mode"
        );

        #[cfg(unix)]
        {
            match mode {
                KillMode::Process => {
                    // Kill only the main process
                    self.kill(pid, signal).await
                }
                KillMode::ProcessGroup => {
                    // Kill the entire process group
                    // Negative PID kills the process group
                    let result = unsafe { libc::kill(-(pid as i32), signal) };
                    if result != 0 {
                        let err = std::io::Error::last_os_error();
                        warn!(
                            pid = pid,
                            signal = signal,
                            mode = %mode,
                            error = %err,
                            "Failed to send signal to process group"
                        );
                        return Err(DomainError::InvalidCommand(format!(
                            "Failed to send signal {} to process group: {}",
                            signal, err
                        )));
                    }
                    debug!(
                        pid = pid,
                        signal = signal,
                        "Signal sent to process group successfully"
                    );
                    Ok(())
                }
                KillMode::ControlGroup => {
                    // Try to kill via cgroup, fallback to process group
                    // For now, fallback to process group (full cgroup implementation would read /sys/fs/cgroup)
                    warn!(
                        pid = pid,
                        "ControlGroup mode not fully implemented, falling back to ProcessGroup"
                    );
                    let result = unsafe { libc::kill(-(pid as i32), signal) };
                    if result != 0 {
                        let err = std::io::Error::last_os_error();
                        return Err(DomainError::InvalidCommand(format!(
                            "Failed to send signal {} to process group: {}",
                            signal, err
                        )));
                    }
                    Ok(())
                }
                KillMode::Mixed => {
                    // Send SIGTERM to main process first
                    info!(pid = pid, "Mixed mode: sending SIGTERM to main process");
                    let _ = self.kill(pid, 15).await; // Ignore errors, continue to kill group

                    // Then send SIGKILL to entire process group
                    tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
                    info!(pid = pid, "Mixed mode: sending SIGKILL to process group");
                    let result = unsafe { libc::kill(-(pid as i32), 9) };
                    if result != 0 {
                        let err = std::io::Error::last_os_error();
                        return Err(DomainError::InvalidCommand(format!(
                            "Failed to send SIGKILL to process group: {}",
                            err
                        )));
                    }
                    debug!(pid = pid, "Mixed mode kill completed");
                    Ok(())
                }
            }
        }

        #[cfg(not(unix))]
        {
            Err(DomainError::InvalidCommand(
                "Process killing not implemented on non-Unix platforms".to_string(),
            ))
        }
    }

    async fn is_running(&self, pid: u32) -> Result<bool, DomainError> {
        #[cfg(unix)]
        {
            // Send signal 0 to check if process exists
            let result = unsafe { libc::kill(pid as i32, 0) };
            Ok(result == 0)
        }

        #[cfg(not(unix))]
        {
            Err(DomainError::InvalidCommand(
                "Process status check not implemented on non-Unix platforms".to_string(),
            ))
        }
    }

    async fn wait_for_exit(&self, pid: u32) -> Result<i32, DomainError> {
        #[cfg(unix)]
        {
            // Poll for process exit
            let mut status: i32 = 0;
            loop {
                let result =
                    unsafe { libc::waitpid(pid as i32, &mut status as *mut i32, libc::WNOHANG) };

                if result == pid as i32 {
                    // Process exited
                    let exit_code = if libc::WIFEXITED(status) {
                        libc::WEXITSTATUS(status)
                    } else {
                        -1
                    };

                    debug!(pid = pid, exit_code = exit_code, "Process exited");
                    return Ok(exit_code);
                } else if result == -1 {
                    let err = std::io::Error::last_os_error();
                    return Err(DomainError::InvalidCommand(format!(
                        "Failed to wait for process: {}",
                        err
                    )));
                }

                // Process still running, wait a bit
                tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
            }
        }

        #[cfg(not(unix))]
        {
            Err(DomainError::InvalidCommand(
                "Process wait not implemented on non-Unix platforms".to_string(),
            ))
        }
    }
}

/// Implementation of resource usage monitoring (not part of ProcessExecutor trait)
impl TokioProcessExecutor {
    /// Get resource usage for a process (reads from cgroups if available)
    pub fn get_resource_usage(&self, pid: Option<u32>) -> Option<ResourceUsage> {
        if !self.cgroup_available {
            return None;
        }

        // Process must be running to have cgroup stats
        let pid = pid?;

        // Cgroups are created as /sys/fs/cgroup/pm-{pid}
        let cgroup_path = Path::new("/sys/fs/cgroup").join(format!("pm-{}", pid));

        if !cgroup_path.exists() {
            return None;
        }

        Some(ResourceUsage {
            memory_current: self.read_memory_current(&cgroup_path).ok(),
            memory_peak: self.read_memory_peak(&cgroup_path).ok(),
            cpu_usage_usec: self.read_cpu_usage_usec(&cgroup_path).ok(),
            cpu_user_usec: self.read_cpu_user_usec(&cgroup_path).ok(),
            cpu_system_usec: self.read_cpu_system_usec(&cgroup_path).ok(),
            pids_current: self.read_pids_current(&cgroup_path).ok(),
        })
    }

    /// Read current memory usage from cgroup
    fn read_memory_current(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let memory_current = cgroup_path.join("memory.current");
        fs::read_to_string(&memory_current)
            .map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to read memory.current: {}", e))
            })?
            .trim()
            .parse()
            .map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to parse memory.current: {}", e))
            })
    }

    /// Read peak memory usage from cgroup
    fn read_memory_peak(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let memory_peak = cgroup_path.join("memory.peak");
        fs::read_to_string(&memory_peak)
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read memory.peak: {}", e)))?
            .trim()
            .parse()
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to parse memory.peak: {}", e)))
    }

    /// Read CPU usage in microseconds from cgroup
    fn read_cpu_usage_usec(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let cpu_stat = cgroup_path.join("cpu.stat");
        let contents = fs::read_to_string(&cpu_stat)
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read cpu.stat: {}", e)))?;

        for line in contents.lines() {
            if let Some(value_str) = line.strip_prefix("usage_usec ") {
                return value_str.parse().map_err(|e| {
                    DomainError::InvalidCommand(format!("Failed to parse usage_usec: {}", e))
                });
            }
        }

        Err(DomainError::InvalidCommand(
            "usage_usec not found in cpu.stat".to_string(),
        ))
    }

    /// Read user CPU time in microseconds from cgroup
    fn read_cpu_user_usec(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let cpu_stat = cgroup_path.join("cpu.stat");
        let contents = fs::read_to_string(&cpu_stat)
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read cpu.stat: {}", e)))?;

        for line in contents.lines() {
            if let Some(value_str) = line.strip_prefix("user_usec ") {
                return value_str.parse().map_err(|e| {
                    DomainError::InvalidCommand(format!("Failed to parse user_usec: {}", e))
                });
            }
        }

        Err(DomainError::InvalidCommand(
            "user_usec not found in cpu.stat".to_string(),
        ))
    }

    /// Read system CPU time in microseconds from cgroup
    fn read_cpu_system_usec(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let cpu_stat = cgroup_path.join("cpu.stat");
        let contents = fs::read_to_string(&cpu_stat)
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read cpu.stat: {}", e)))?;

        for line in contents.lines() {
            if let Some(value_str) = line.strip_prefix("system_usec ") {
                return value_str.parse().map_err(|e| {
                    DomainError::InvalidCommand(format!("Failed to parse system_usec: {}", e))
                });
            }
        }

        Err(DomainError::InvalidCommand(
            "system_usec not found in cpu.stat".to_string(),
        ))
    }

    /// Read current number of PIDs from cgroup
    fn read_pids_current(&self, cgroup_path: &Path) -> Result<u32, DomainError> {
        let pids_current = cgroup_path.join("pids.current");
        fs::read_to_string(&pids_current)
            .map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to read pids.current: {}", e))
            })?
            .trim()
            .parse()
            .map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to parse pids.current: {}", e))
            })
    }
}

/// Implement ResourceUsageReader trait for TokioProcessExecutor
impl ResourceUsageReader for TokioProcessExecutor {
    fn get_usage(&self, pid: Option<u32>) -> Option<ResourceUsage> {
        self.get_resource_usage(pid)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_spawn_simple_process() {
        let executor = TokioProcessExecutor::new();

        let config = SpawnConfig {
            command: "/bin/echo".to_string(),
            args: vec!["hello".to_string()],
            env_vars: vec![],
            working_dir: None,
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            user: None,
            group: None,
            ambient_capabilities: vec![],
            resource_limits: crate::domain::ResourceLimits::default(),
            listen_fds: Vec::new(),
        };

        let result = executor.spawn(config).await;
        assert!(result.is_ok());

        let spawn_result = result.unwrap();
        assert!(spawn_result.pid > 0);
    }

    #[tokio::test]
    async fn test_spawn_invalid_command() {
        let executor = TokioProcessExecutor::new();

        let config = SpawnConfig {
            command: "/nonexistent/command".to_string(),
            args: vec![],
            env_vars: vec![],
            working_dir: None,
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            user: None,
            group: None,
            ambient_capabilities: vec![],
            resource_limits: crate::domain::ResourceLimits::default(),
            listen_fds: Vec::new(),
        };

        let result = executor.spawn(config).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    #[cfg(unix)]
    async fn test_is_running() {
        let executor = TokioProcessExecutor::new();

        // Spawn a long-running process
        let config = SpawnConfig {
            command: "/bin/sleep".to_string(),
            args: vec!["2".to_string()],
            env_vars: vec![],
            working_dir: None,
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            user: None,
            group: None,
            ambient_capabilities: vec![],
            resource_limits: crate::domain::ResourceLimits::default(),
            listen_fds: Vec::new(),
        };

        let result = executor.spawn(config).await.unwrap();
        let pid = result.pid;

        // Check if running
        let is_running = executor.is_running(pid).await.unwrap();
        assert!(is_running);

        // Kill it
        let _ = executor.kill(pid, 9).await;

        // Give it a moment
        tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;

        // Should not be running
        let is_running = executor.is_running(pid).await.unwrap();
        assert!(!is_running);
    }

    #[tokio::test]
    #[cfg(unix)]
    async fn test_kill_process() {
        let executor = TokioProcessExecutor::new();

        // Spawn a long-running process
        let config = SpawnConfig {
            command: "/bin/sleep".to_string(),
            args: vec!["10".to_string()],
            env_vars: vec![],
            working_dir: None,
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            user: None,
            group: None,
            ambient_capabilities: vec![],
            resource_limits: crate::domain::ResourceLimits::default(),
            listen_fds: Vec::new(),
        };

        let result = executor.spawn(config).await.unwrap();
        let pid = result.pid;

        // Kill it
        let kill_result = executor.kill(pid, 15).await;
        assert!(kill_result.is_ok());
    }

    #[tokio::test]
    async fn test_spawn_with_env_vars() {
        let executor = TokioProcessExecutor::new();

        let config = SpawnConfig {
            command: "/usr/bin/env".to_string(),
            args: vec![],
            env_vars: vec![
                ("TEST_VAR".to_string(), "test_value".to_string()),
                ("ANOTHER_VAR".to_string(), "another_value".to_string()),
            ],
            working_dir: None,
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            user: None,
            group: None,
            ambient_capabilities: vec![],
            resource_limits: crate::domain::ResourceLimits::default(),
            listen_fds: Vec::new(),
        };

        let result = executor.spawn(config).await;
        assert!(result.is_ok());

        let spawn_result = result.unwrap();
        assert!(spawn_result.pid > 0);
    }

    #[tokio::test]
    async fn test_spawn_with_working_dir() {
        let executor = TokioProcessExecutor::new();

        let config = SpawnConfig {
            command: "/bin/pwd".to_string(),
            args: vec![],
            env_vars: vec![],
            working_dir: Some("/tmp".to_string()),
            environment_file: None,
            pidfile: None,
            stdout: None,
            stderr: None,
            user: None,
            group: None,
            ambient_capabilities: vec![],
            resource_limits: crate::domain::ResourceLimits::default(),
            listen_fds: Vec::new(),
        };

        let result = executor.spawn(config).await;
        assert!(result.is_ok());

        let spawn_result = result.unwrap();
        assert!(spawn_result.pid > 0);
    }
}
