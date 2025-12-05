//! Tokio Process Executor
//! Real implementation of ProcessExecutor port using tokio
//!
//! This module provides cross-platform process management:
//! - Linux: Uses cgroups v2 for resource limits, process groups, capabilities
//! - Windows: Uses Job Objects for resource limits and process management

use crate::domain::{
    ports::{ProcessExecutor, SpawnConfig, SpawnResult},
    use_cases::ResourceUsageReader,
    DomainError, ResourceUsage,
};
use async_trait::async_trait;
use std::collections::HashMap;
use std::fs::{self, File, OpenOptions};
use std::io::{BufRead, BufReader};
use std::path::Path;
use std::process::{Command, Stdio};
use tracing::{debug, error, info, warn};

// Platform-specific imports
#[cfg(unix)]
use std::os::unix::process::CommandExt;

#[cfg(windows)]
use std::os::windows::process::CommandExt as WindowsCommandExt;

/// Tokio-based process executor
///
/// This adapter translates domain operations into actual system calls.
/// On Windows, it uses Job Objects for process grouping and resource limits.
/// On Linux, it uses cgroups v2 and process groups.
pub struct TokioProcessExecutor {
    /// Whether cgroup v2 is available (Unix - only meaningful on Linux)
    #[cfg(unix)]
    cgroup_available: bool,

    /// Whether Job Objects are available (Windows only)
    #[cfg(windows)]
    job_objects_available: bool,
}

impl TokioProcessExecutor {
    pub fn new() -> Self {
        #[cfg(unix)]
        {
            let cgroup_available = Self::detect_cgroup_v2();

            if cgroup_available {
                info!("cgroup v2 detected and available for resource limits");
            } else {
                warn!("cgroup v2 not available, will use rlimit fallback for resource limits");
            }

            Self { cgroup_available }
        }

        #[cfg(windows)]
        {
            info!("Windows Job Objects available for process management");
            Self {
                job_objects_available: true,
            }
        }

        #[cfg(not(any(unix, windows)))]
        {
            warn!("Resource limits not supported on this platform");
            Self {}
        }
    }

    /// Detect if cgroup v2 is available and usable
    /// Only returns true on Linux with cgroup v2 support
    #[cfg(unix)]
    fn detect_cgroup_v2() -> bool {
        #[cfg(target_os = "linux")]
        {
            let cgroup_path = std::path::Path::new("/sys/fs/cgroup");

            if !cgroup_path.exists() {
                return false;
            }

            let controllers_file = cgroup_path.join("cgroup.controllers");
            if !controllers_file.exists() {
                return false;
            }

            if let Ok(controllers) = std::fs::read_to_string(&controllers_file) {
                let has_cpu = controllers.contains("cpu");
                let has_memory = controllers.contains("memory");
                let has_pids = controllers.contains("pids");
                return has_cpu || has_memory || has_pids;
            }

            false
        }

        #[cfg(not(target_os = "linux"))]
        {
            // cgroups are Linux-specific
            false
        }
    }
}

impl Default for TokioProcessExecutor {
    fn default() -> Self {
        Self::new()
    }
}

// ============================================================================
// Common Helper Functions (Cross-Platform)
// ============================================================================

impl TokioProcessExecutor {
    /// Load environment variables from a file
    /// Format: KEY=VALUE, one per line, # for comments
    ///
    /// Supports systemd-style optional prefix:
    /// - If path starts with '-', the file is optional (no error if missing)
    fn load_env_file(path: &str) -> Result<HashMap<String, String>, DomainError> {
        let (actual_path, optional) = if let Some(stripped) = path.strip_prefix('-') {
            (stripped, true)
        } else {
            (path, false)
        };

        let file = match File::open(actual_path) {
            Ok(f) => f,
            Err(e) => {
                if optional {
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

            if line.is_empty() || line.starts_with('#') {
                continue;
            }

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
}

// ============================================================================
// Unix-Specific Implementation (Linux, macOS, etc.)
// ============================================================================

#[cfg(unix)]
impl TokioProcessExecutor {
    /// Apply resource limits using cgroups v2 (Linux only)
    /// Note: This is called from spawn() on Linux but not on other platforms
    #[cfg(target_os = "linux")]
    #[allow(dead_code)]
    fn apply_resource_limits(
        pid: u32,
        limits: &crate::domain::ResourceLimits,
    ) -> Result<(), DomainError> {
        debug!(
            pid = pid,
            limits = %limits,
            "Applying resource limits"
        );

        let cgroup_root = std::path::Path::new("/sys/fs/cgroup");
        if !cgroup_root.exists() {
            debug!("cgroups not available, skipping resource limits");
            return Ok(());
        }

        let cgroup_path = format!("/sys/fs/cgroup/pm-{}", pid);
        let cgroup_dir = std::path::Path::new(&cgroup_path);

        if let Err(e) = std::fs::create_dir_all(cgroup_dir) {
            warn!(
                pid = pid,
                error = %e,
                "Failed to create cgroup directory (insufficient permissions?)"
            );
            return Ok(());
        }

        let procs_path = cgroup_dir.join("cgroup.procs");
        if let Err(e) = std::fs::write(&procs_path, format!("{}", pid)) {
            warn!(
                pid = pid,
                error = %e,
                "Failed to add process to cgroup"
            );
            let _ = std::fs::remove_dir(cgroup_dir);
            return Ok(());
        }

        // Apply CPU limit
        if let Some(cpu_millis) = limits.cpu_millis {
            let period = 100_000;
            let quota = (cpu_millis * period) / 1000;
            let cpu_max_path = cgroup_dir.join("cpu.max");
            let cpu_max_value = format!("{} {}", quota, period);

            if let Err(e) = std::fs::write(&cpu_max_path, &cpu_max_value) {
                warn!(pid = pid, error = %e, "Failed to set CPU limit");
            } else {
                debug!(pid = pid, cpu_millis = cpu_millis, "Applied CPU limit");
            }
        }

        // Apply memory limit
        if let Some(memory_bytes) = limits.memory_bytes {
            let memory_max_path = cgroup_dir.join("memory.max");
            if let Err(e) = std::fs::write(&memory_max_path, format!("{}", memory_bytes)) {
                warn!(pid = pid, error = %e, "Failed to set memory limit");
            } else {
                debug!(pid = pid, memory_bytes = memory_bytes, "Applied memory limit");
            }
        }

        // Apply PIDs limit
        if let Some(max_pids) = limits.max_pids {
            let pids_max_path = cgroup_dir.join("pids.max");
            if let Err(e) = std::fs::write(&pids_max_path, format!("{}", max_pids)) {
                warn!(pid = pid, error = %e, "Failed to set PIDs limit");
            } else {
                debug!(pid = pid, max_pids = max_pids, "Applied PIDs limit");
            }
        }

        Ok(())
    }

    /// Apply resource limits stub for non-Linux Unix (cgroups not available)
    #[cfg(not(target_os = "linux"))]
    #[allow(dead_code)]
    fn apply_resource_limits(
        pid: u32,
        limits: &crate::domain::ResourceLimits,
    ) -> Result<(), DomainError> {
        let _ = (pid, limits);
        debug!(
            pid = pid,
            limits = %limits,
            "cgroups not available on this platform, skipping resource limits"
        );
        Ok(())
    }

    /// Apply rlimits from within child process (fallback when cgroups unavailable)
    fn apply_rlimits_in_child(limits: &crate::domain::ResourceLimits) -> std::io::Result<()> {
        use libc::{rlimit, setrlimit, RLIMIT_AS, RLIMIT_CPU};

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

        Ok(())
    }

    /// Resolve username to UID (all Unix platforms)
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

    /// Resolve group name to GID (all Unix platforms)
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

    /// Apply ambient capabilities (Linux-only)
    #[cfg(target_os = "linux")]
    #[allow(dead_code)]
    fn apply_ambient_capabilities(capabilities: &[String]) -> std::io::Result<()> {
        use caps::{CapSet, Capability};

        for cap_str in capabilities {
            let capability = cap_str.parse::<Capability>().map_err(|e| {
                std::io::Error::new(
                    std::io::ErrorKind::InvalidInput,
                    format!("Invalid capability '{}': {}", cap_str, e),
                )
            })?;

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

    /// Stub for ambient capabilities on non-Linux Unix (not supported)
    #[cfg(not(target_os = "linux"))]
    #[allow(dead_code)]
    fn apply_ambient_capabilities(capabilities: &[String]) -> std::io::Result<()> {
        if !capabilities.is_empty() {
            tracing::warn!("Ambient capabilities are only supported on Linux");
        }
        Ok(())
    }
}

// ============================================================================
// Windows-Specific Implementation
// ============================================================================

#[cfg(windows)]
impl TokioProcessExecutor {
    /// Apply resource limits using Windows Job Objects
    fn apply_resource_limits_windows(
        pid: u32,
        limits: &crate::domain::ResourceLimits,
    ) -> Result<(), DomainError> {
        use windows::Win32::Foundation::{CloseHandle, HANDLE};
        use windows::Win32::System::JobObjects::{
            AssignProcessToJobObject, CreateJobObjectW, JobObjectExtendedLimitInformation,
            SetInformationJobObject, JOBOBJECT_BASIC_LIMIT_INFORMATION,
            JOBOBJECT_EXTENDED_LIMIT_INFORMATION, JOB_OBJECT_LIMIT_JOB_MEMORY,
            JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, JOB_OBJECT_LIMIT_PROCESS_MEMORY,
        };
        use windows::Win32::System::Threading::{OpenProcess, PROCESS_ALL_ACCESS};

        debug!(
            pid = pid,
            limits = %limits,
            "Applying resource limits via Windows Job Object"
        );

        unsafe {
            // Create a job object for this process
            let job = CreateJobObjectW(None, None).map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to create job object: {}", e))
            })?;

            // Configure job limits
            let mut info: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();

            // Always set KILL_ON_JOB_CLOSE to ensure child processes die with parent
            info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;

            // Apply memory limit
            if let Some(memory_bytes) = limits.memory_bytes {
                info.BasicLimitInformation.LimitFlags |= JOB_OBJECT_LIMIT_PROCESS_MEMORY;
                info.ProcessMemoryLimit = memory_bytes as usize;
            }

            // Set the job limits
            let result = SetInformationJobObject(
                job,
                JobObjectExtendedLimitInformation,
                &info as *const _ as *const std::ffi::c_void,
                std::mem::size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
            );

            if result.is_err() {
                let _ = CloseHandle(job);
                warn!(pid = pid, "Failed to set job object limits");
                return Ok(());
            }

            // Open the process and assign it to the job
            let process = OpenProcess(PROCESS_ALL_ACCESS, false, pid);
            match process {
                Ok(handle) => {
                    let assign_result = AssignProcessToJobObject(job, handle);
                    let _ = CloseHandle(handle);

                    if assign_result.is_err() {
                        let _ = CloseHandle(job);
                        warn!(pid = pid, "Failed to assign process to job object");
                        return Ok(());
                    }

                    debug!(pid = pid, "Process assigned to job object with limits");

                    // Note: We intentionally don't close the job handle here
                    // It needs to stay open for the limits to remain in effect
                    // The handle will be closed when the process manager exits
                }
                Err(e) => {
                    let _ = CloseHandle(job);
                    warn!(pid = pid, error = %e, "Failed to open process for job assignment");
                }
            }
        }

        Ok(())
    }
}

// ============================================================================
// ProcessExecutor Trait Implementation
// ============================================================================

#[async_trait]
impl ProcessExecutor for TokioProcessExecutor {
    async fn spawn(&self, config: SpawnConfig) -> Result<SpawnResult, DomainError> {
        info!(
            command = %config.command,
            args = ?config.args,
            "Spawning process"
        );

        if config.command.is_empty() {
            return Err(DomainError::InvalidCommand("Empty command".to_string()));
        }

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

        // Add explicit environment variables
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

        // Configure stdio
        cmd.stdin(Stdio::null());
        cmd.stdout(Self::configure_stdout(config.stdout.as_deref())?);
        cmd.stderr(Self::configure_stderr(config.stderr.as_deref())?);

        // Platform-specific spawn configuration
        #[cfg(unix)]
        {
            self.configure_unix_spawn(&mut cmd, &config)?;
        }

        #[cfg(windows)]
        {
            self.configure_windows_spawn(&mut cmd, &config)?;
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

        // Apply resource limits post-spawn
        #[cfg(target_os = "linux")]
        {
            if self.cgroup_available && config.resource_limits.has_limits() {
                if let Err(e) = Self::apply_resource_limits(pid, &config.resource_limits) {
                    warn!(
                        pid = pid,
                        error = %e,
                        "Failed to apply cgroup resource limits (continuing anyway)"
                    );
                }
            }
        }

        #[cfg(windows)]
        {
            if self.job_objects_available && config.resource_limits.has_limits() {
                if let Err(e) = Self::apply_resource_limits_windows(pid, &config.resource_limits) {
                    warn!(
                        pid = pid,
                        error = %e,
                        "Failed to apply job object resource limits (continuing anyway)"
                    );
                }
            }
        }

        info!(pid = pid, "Process spawned successfully");

        // Create exit handle for monitoring
        let exit_handle = self.create_exit_handle(child, pid);

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

        #[cfg(windows)]
        {
            self.terminate_process_windows(pid, signal).await
        }

        #[cfg(not(any(unix, windows)))]
        {
            Err(DomainError::InvalidCommand(
                "Process killing not implemented on this platform".to_string(),
            ))
        }
    }

    async fn kill_with_mode(
        &self,
        pid: u32,
        signal: i32,
        mode: crate::domain::KillMode,
    ) -> Result<(), DomainError> {
        info!(
            pid = pid,
            signal = signal,
            mode = %mode,
            "Killing process with mode"
        );

        #[cfg(unix)]
        {
            self.kill_with_mode_unix(pid, signal, mode).await
        }

        #[cfg(windows)]
        {
            // On Windows, we primarily use TerminateProcess
            // Job Objects handle child process termination automatically
            self.terminate_process_windows(pid, signal).await
        }

        #[cfg(not(any(unix, windows)))]
        {
            Err(DomainError::InvalidCommand(
                "Process killing not implemented on this platform".to_string(),
            ))
        }
    }

    async fn is_running(&self, pid: u32) -> Result<bool, DomainError> {
        #[cfg(unix)]
        {
            let result = unsafe { libc::kill(pid as i32, 0) };
            Ok(result == 0)
        }

        #[cfg(windows)]
        {
            self.is_process_running_windows(pid)
        }

        #[cfg(not(any(unix, windows)))]
        {
            Err(DomainError::InvalidCommand(
                "Process status check not implemented on this platform".to_string(),
            ))
        }
    }

    async fn wait_for_exit(&self, pid: u32) -> Result<i32, DomainError> {
        #[cfg(unix)]
        {
            self.wait_for_exit_unix(pid).await
        }

        #[cfg(windows)]
        {
            self.wait_for_exit_windows(pid).await
        }

        #[cfg(not(any(unix, windows)))]
        {
            Err(DomainError::InvalidCommand(
                "Process wait not implemented on this platform".to_string(),
            ))
        }
    }
}

// ============================================================================
// Unix-Specific Spawn and Kill Implementation
// ============================================================================

#[cfg(unix)]
impl TokioProcessExecutor {
    fn configure_unix_spawn(
        &self,
        cmd: &mut Command,
        config: &SpawnConfig,
    ) -> Result<(), DomainError> {
        // Resolve user/group to UID/GID upfront
        let uid_opt = if let Some(ref user) = config.user {
            let uid = Self::get_uid(user)?;
            debug!(user = %user, uid = uid, "Resolved process user");
            Some(uid)
        } else {
            None
        };

        let gid_opt = if let Some(ref group) = config.group {
            let gid = Self::get_gid(group)?;
            debug!(group = %group, gid = gid, "Resolved process group");
            Some(gid)
        } else {
            None
        };

        let use_rlimit = !self.cgroup_available && config.resource_limits.has_limits();
        #[cfg(target_os = "linux")]
        let has_capabilities = !config.ambient_capabilities.is_empty();

        let limits = config.resource_limits.clone();
        #[cfg(target_os = "linux")]
        let capabilities = config.ambient_capabilities.clone();
        let socket_fds = config.listen_fds.clone();
        let fd_env_var_names = config.fd_env_var_names.clone();
        let has_socket_fds = !socket_fds.is_empty();

        // Socket activation environment
        if has_socket_fds {
            let num_fds = socket_fds.len();
            debug!(num_fds = num_fds, "Setting up socket activation");

            // Always set LISTEN_FDS for systemd compatibility
            cmd.env("LISTEN_FDS", num_fds.to_string());
            cmd.env("LISTEN_PID", std::process::id().to_string());

            // Set custom env vars for each FD (Datadog-style)
            for (i, fd_env_name) in fd_env_var_names.iter().enumerate() {
                if !fd_env_name.is_empty() {
                    let target_fd = 3 + i;
                    cmd.env(fd_env_name, target_fd.to_string());
                    info!(
                        env_var = %fd_env_name,
                        fd = target_fd,
                        "Socket activation: setting {} = {}",
                        fd_env_name,
                        target_fd
                    );
                }
            }

            info!(
                num_fds = num_fds,
                "Socket activation: passing {} file descriptor(s) starting at FD 3",
                num_fds
            );
        }

        unsafe {
            cmd.pre_exec(move || {
                // Create new session/process group
                if libc::setsid() < 0 {
                    // Ignore error if already session leader
                }

                // Duplicate socket FDs for socket activation
                if has_socket_fds {
                    for (i, &fd) in socket_fds.iter().enumerate() {
                        let target_fd = 3 + i as i32;
                        if libc::dup2(fd, target_fd) == -1 {
                            return Err(std::io::Error::last_os_error());
                        }
                    }
                }

                // Apply rlimits (fallback if cgroups unavailable)
                if use_rlimit {
                    Self::apply_rlimits_in_child(&limits)?;
                }

                // Linux-specific: capabilities and PDEATHSIG
                #[cfg(target_os = "linux")]
                {
                    let needs_caps_after_setuid = has_capabilities && uid_opt.is_some();

                    if has_capabilities {
                        const PR_SET_SECUREBITS: libc::c_int = 28;
                        const SECBIT_KEEP_CAPS: libc::c_ulong = 0x10;
                        if libc::prctl(PR_SET_SECUREBITS, SECBIT_KEEP_CAPS) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }
                        Self::apply_ambient_capabilities(&capabilities)?;
                    }

                    // Set group (before setuid)
                    if let Some(gid) = gid_opt {
                        if libc::setgid(gid) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }
                    }

                    // Set user
                    if let Some(uid) = uid_opt {
                        if libc::setuid(uid) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }
                    }

                    // Re-raise ambient capabilities after setuid
                    if needs_caps_after_setuid {
                        Self::apply_ambient_capabilities(&capabilities)?;
                    }

                    // Set parent death signal (after setuid)
                    const PR_SET_PDEATHSIG: libc::c_int = 1;
                    if libc::prctl(PR_SET_PDEATHSIG, libc::SIGKILL) != 0 {
                        return Err(std::io::Error::last_os_error());
                    }
                }

                // Non-Linux Unix: just set user/group
                #[cfg(all(unix, not(target_os = "linux")))]
                {
                    if let Some(gid) = gid_opt {
                        if libc::setgid(gid) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }
                    }
                    if let Some(uid) = uid_opt {
                        if libc::setuid(uid) != 0 {
                            return Err(std::io::Error::last_os_error());
                        }
                    }
                }

                Ok(())
            });
        }

        Ok(())
    }

    async fn kill_with_mode_unix(
        &self,
        pid: u32,
        signal: i32,
        mode: crate::domain::KillMode,
    ) -> Result<(), DomainError> {
        match mode {
            crate::domain::KillMode::Process => self.kill(pid, signal).await,
            crate::domain::KillMode::ProcessGroup => {
                let result = unsafe { libc::kill(-(pid as i32), signal) };
                if result != 0 {
                    let err = std::io::Error::last_os_error();
                    warn!(
                        pid = pid,
                        signal = signal,
                        error = %err,
                        "Failed to send signal to process group"
                    );
                    return Err(DomainError::InvalidCommand(format!(
                        "Failed to send signal {} to process group: {}",
                        signal, err
                    )));
                }
                debug!(pid = pid, signal = signal, "Signal sent to process group");
                Ok(())
            }
            crate::domain::KillMode::ControlGroup => {
                let cgroup_path = format!("/sys/fs/cgroup/pm-{}/cgroup.procs", pid);
                if let Ok(contents) = std::fs::read_to_string(&cgroup_path) {
                    let mut kill_count = 0;
                    for line in contents.lines() {
                        if let Ok(cgroup_pid) = line.trim().parse::<u32>() {
                            let result = unsafe { libc::kill(cgroup_pid as i32, signal) };
                            if result == 0 {
                                kill_count += 1;
                            }
                        }
                    }
                    debug!(pid = pid, signal = signal, kill_count = kill_count, "Killed cgroup members");
                    Ok(())
                } else {
                    // Fallback to process group
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
            }
            crate::domain::KillMode::Mixed => {
                let _ = self.kill(pid, 15).await;
                tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
                let result = unsafe { libc::kill(-(pid as i32), 9) };
                if result != 0 {
                    let err = std::io::Error::last_os_error();
                    return Err(DomainError::InvalidCommand(format!(
                        "Failed to send SIGKILL to process group: {}",
                        err
                    )));
                }
                Ok(())
            }
        }
    }

    async fn wait_for_exit_unix(&self, pid: u32) -> Result<i32, DomainError> {
        let mut status: i32 = 0;
        loop {
            let result =
                unsafe { libc::waitpid(pid as i32, &mut status as *mut i32, libc::WNOHANG) };

            if result == pid as i32 {
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

            tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
        }
    }

    fn create_exit_handle(
        &self,
        mut child: std::process::Child,
        pid: u32,
    ) -> Option<crate::domain::ports::ProcessExitHandle> {
        use crate::domain::ports::ProcessExitHandle;
        let (tx, rx) = tokio::sync::oneshot::channel();

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
            let _ = tx.send(exit_result);
        });

        let exit_fut = async move {
            match rx.await {
                Ok(result) => result,
                Err(_) => Err(DomainError::InvalidCommand(
                    "Process monitor task died unexpectedly".to_string(),
                )),
            }
        };
        Some(Box::pin(exit_fut) as ProcessExitHandle)
    }
}

// ============================================================================
// Windows-Specific Spawn and Kill Implementation
// ============================================================================

#[cfg(windows)]
impl TokioProcessExecutor {
    fn configure_windows_spawn(
        &self,
        cmd: &mut Command,
        config: &SpawnConfig,
    ) -> Result<(), DomainError> {
        use windows::Win32::System::Threading::CREATE_NEW_PROCESS_GROUP;

        // Create process in a new process group for easier management
        cmd.creation_flags(CREATE_NEW_PROCESS_GROUP.0);

        // User/group switching is not directly supported on Windows
        // Would require impersonation which is complex and rarely needed
        if config.user.is_some() || config.group.is_some() {
            warn!(
                "User/group specification is not supported on Windows, running as current user"
            );
        }

        // Capabilities are Linux-specific
        if !config.ambient_capabilities.is_empty() {
            warn!("Ambient capabilities are Linux-specific and ignored on Windows");
        }

        // Socket activation on Windows uses handle inheritance
        if !config.listen_fds.is_empty() {
            // For Windows, we'd need to convert FDs to handles and use
            // SetHandleInformation to make them inheritable
            // This is a complex topic - for now, log a warning
            warn!(
                "Socket activation with FD passing is limited on Windows; use TCP sockets instead"
            );
        }

        Ok(())
    }

    async fn terminate_process_windows(&self, pid: u32, _signal: i32) -> Result<(), DomainError> {
        use windows::Win32::Foundation::CloseHandle;
        use windows::Win32::System::Threading::{
            OpenProcess, TerminateProcess, PROCESS_TERMINATE,
        };

        unsafe {
            let process = OpenProcess(PROCESS_TERMINATE, false, pid).map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to open process {}: {}", pid, e))
            })?;

            let result = TerminateProcess(process, 1);
            let _ = CloseHandle(process);

            if result.is_err() {
                return Err(DomainError::InvalidCommand(format!(
                    "Failed to terminate process {}",
                    pid
                )));
            }
        }

        debug!(pid = pid, "Process terminated");
        Ok(())
    }

    fn is_process_running_windows(&self, pid: u32) -> Result<bool, DomainError> {
        use windows::Win32::Foundation::{CloseHandle, WAIT_TIMEOUT};
        use windows::Win32::System::Threading::{
            OpenProcess, WaitForSingleObject, PROCESS_SYNCHRONIZE,
        };

        unsafe {
            let process = match OpenProcess(PROCESS_SYNCHRONIZE, false, pid) {
                Ok(h) => h,
                Err(_) => return Ok(false), // Process doesn't exist
            };

            let result = WaitForSingleObject(process, 0);
            let _ = CloseHandle(process);

            Ok(result == WAIT_TIMEOUT)
        }
    }

    async fn wait_for_exit_windows(&self, pid: u32) -> Result<i32, DomainError> {
        use windows::Win32::Foundation::{CloseHandle, WAIT_OBJECT_0};
        use windows::Win32::System::Threading::{
            GetExitCodeProcess, OpenProcess, WaitForSingleObject, INFINITE,
            PROCESS_QUERY_INFORMATION, PROCESS_SYNCHRONIZE,
        };

        let pid_copy = pid;
        let result = tokio::task::spawn_blocking(move || unsafe {
            let process = OpenProcess(
                PROCESS_SYNCHRONIZE | PROCESS_QUERY_INFORMATION,
                false,
                pid_copy,
            )
            .map_err(|e| {
                DomainError::InvalidCommand(format!("Failed to open process {}: {}", pid_copy, e))
            })?;

            let wait_result = WaitForSingleObject(process, INFINITE);
            if wait_result != WAIT_OBJECT_0 {
                let _ = CloseHandle(process);
                return Err(DomainError::InvalidCommand(format!(
                    "Failed to wait for process {}",
                    pid_copy
                )));
            }

            let mut exit_code: u32 = 0;
            let code_result = GetExitCodeProcess(process, &mut exit_code);
            let _ = CloseHandle(process);

            if code_result.is_err() {
                return Err(DomainError::InvalidCommand(format!(
                    "Failed to get exit code for process {}",
                    pid_copy
                )));
            }

            Ok(exit_code as i32)
        })
        .await
        .map_err(|e| DomainError::InvalidCommand(format!("Wait task failed: {}", e)))?;

        result
    }

    fn create_exit_handle(
        &self,
        mut child: std::process::Child,
        pid: u32,
    ) -> Option<crate::domain::ports::ProcessExitHandle> {
        use crate::domain::ports::ProcessExitHandle;
        let (tx, rx) = tokio::sync::oneshot::channel();

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
            let _ = tx.send(exit_result);
        });

        let exit_fut = async move {
            match rx.await {
                Ok(result) => result,
                Err(_) => Err(DomainError::InvalidCommand(
                    "Process monitor task died unexpectedly".to_string(),
                )),
            }
        };
        Some(Box::pin(exit_fut) as ProcessExitHandle)
    }
}

// ============================================================================
// Resource Usage Monitoring
// ============================================================================

impl TokioProcessExecutor {
    /// Get resource usage for a process
    #[cfg(target_os = "linux")]
    pub fn get_resource_usage(&self, pid: Option<u32>) -> Option<ResourceUsage> {
        if !self.cgroup_available {
            return None;
        }

        let pid = pid?;
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

    #[cfg(windows)]
    pub fn get_resource_usage(&self, pid: Option<u32>) -> Option<ResourceUsage> {
        use windows::Win32::Foundation::CloseHandle;
        use windows::Win32::System::ProcessStatus::{
            GetProcessMemoryInfo, PROCESS_MEMORY_COUNTERS,
        };
        use windows::Win32::System::Threading::{OpenProcess, PROCESS_QUERY_INFORMATION, PROCESS_VM_READ};

        let pid = pid?;

        unsafe {
            let process = OpenProcess(
                PROCESS_QUERY_INFORMATION | PROCESS_VM_READ,
                false,
                pid,
            ).ok()?;

            let mut mem_info: PROCESS_MEMORY_COUNTERS = std::mem::zeroed();
            mem_info.cb = std::mem::size_of::<PROCESS_MEMORY_COUNTERS>() as u32;

            let result = GetProcessMemoryInfo(
                process,
                &mut mem_info,
                std::mem::size_of::<PROCESS_MEMORY_COUNTERS>() as u32,
            );

            let _ = CloseHandle(process);

            if result.is_err() {
                return None;
            }

            Some(ResourceUsage {
                memory_current: Some(mem_info.WorkingSetSize as u64),
                memory_peak: Some(mem_info.PeakWorkingSetSize as u64),
                cpu_usage_usec: None, // Would need GetProcessTimes
                cpu_user_usec: None,
                cpu_system_usec: None,
                pids_current: None, // Not easily available on Windows
            })
        }
    }

    #[cfg(not(any(target_os = "linux", windows)))]
    pub fn get_resource_usage(&self, _pid: Option<u32>) -> Option<ResourceUsage> {
        None
    }

    #[cfg(target_os = "linux")]
    fn read_memory_current(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        fs::read_to_string(cgroup_path.join("memory.current"))
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read memory.current: {}", e)))?
            .trim()
            .parse()
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to parse memory.current: {}", e)))
    }

    #[cfg(target_os = "linux")]
    fn read_memory_peak(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        fs::read_to_string(cgroup_path.join("memory.peak"))
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read memory.peak: {}", e)))?
            .trim()
            .parse()
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to parse memory.peak: {}", e)))
    }

    #[cfg(target_os = "linux")]
    fn read_cpu_usage_usec(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let contents = fs::read_to_string(cgroup_path.join("cpu.stat"))
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read cpu.stat: {}", e)))?;

        for line in contents.lines() {
            if let Some(value_str) = line.strip_prefix("usage_usec ") {
                return value_str.parse().map_err(|e| {
                    DomainError::InvalidCommand(format!("Failed to parse usage_usec: {}", e))
                });
            }
        }
        Err(DomainError::InvalidCommand("usage_usec not found".to_string()))
    }

    #[cfg(target_os = "linux")]
    fn read_cpu_user_usec(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let contents = fs::read_to_string(cgroup_path.join("cpu.stat"))
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read cpu.stat: {}", e)))?;

        for line in contents.lines() {
            if let Some(value_str) = line.strip_prefix("user_usec ") {
                return value_str.parse().map_err(|e| {
                    DomainError::InvalidCommand(format!("Failed to parse user_usec: {}", e))
                });
            }
        }
        Err(DomainError::InvalidCommand("user_usec not found".to_string()))
    }

    #[cfg(target_os = "linux")]
    fn read_cpu_system_usec(&self, cgroup_path: &Path) -> Result<u64, DomainError> {
        let contents = fs::read_to_string(cgroup_path.join("cpu.stat"))
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read cpu.stat: {}", e)))?;

        for line in contents.lines() {
            if let Some(value_str) = line.strip_prefix("system_usec ") {
                return value_str.parse().map_err(|e| {
                    DomainError::InvalidCommand(format!("Failed to parse system_usec: {}", e))
                });
            }
        }
        Err(DomainError::InvalidCommand("system_usec not found".to_string()))
    }

    #[cfg(target_os = "linux")]
    fn read_pids_current(&self, cgroup_path: &Path) -> Result<u32, DomainError> {
        fs::read_to_string(cgroup_path.join("pids.current"))
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to read pids.current: {}", e)))?
            .trim()
            .parse()
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to parse pids.current: {}", e)))
    }
}

impl ResourceUsageReader for TokioProcessExecutor {
    fn get_usage(&self, pid: Option<u32>) -> Option<ResourceUsage> {
        self.get_resource_usage(pid)
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_spawn_simple_process() {
        let executor = TokioProcessExecutor::new();

        #[cfg(unix)]
        let command = "/bin/echo".to_string();
        #[cfg(windows)]
        let command = "cmd".to_string();
        #[cfg(windows)]
        let args = vec!["/c".to_string(), "echo".to_string(), "hello".to_string()];
        #[cfg(unix)]
        let args = vec!["hello".to_string()];

        let config = SpawnConfig {
            command,
            args,
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
            fd_env_var_names: Vec::new(),
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
            fd_env_var_names: Vec::new(),
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
            fd_env_var_names: Vec::new(),
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
    async fn test_spawn_with_env_vars() {
        let executor = TokioProcessExecutor::new();

        #[cfg(unix)]
        let (command, args) = ("/usr/bin/env".to_string(), vec![]);
        #[cfg(windows)]
        let (command, args) = ("cmd".to_string(), vec!["/c".to_string(), "set".to_string()]);

        let config = SpawnConfig {
            command,
            args,
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
            fd_env_var_names: Vec::new(),
        };

        let result = executor.spawn(config).await;
        assert!(result.is_ok());

        let spawn_result = result.unwrap();
        assert!(spawn_result.pid > 0);
    }
}
