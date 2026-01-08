//! Mappers between protobuf types and domain types

use crate::domain::{
    HealthCheck, HealthCheckType, KillMode as DomainKillMode, PathCondition, Process, ProcessId,
    ProcessState as DomainProcessState, ProcessType as DomainProcessType, ResourceLimits,
    RestartPolicy as DomainRestartPolicy,
};
use crate::proto::process_manager::{
    HealthCheck as ProtoHealthCheck, HealthCheckType as ProtoHealthCheckType,
    KillMode as ProtoKillMode, Process as ProtoProcess, ProcessState as ProtoProcessState,
    ProcessType as ProtoProcessType, ResourceLimits as ProtoResourceLimits,
    RestartPolicy as ProtoRestartPolicy,
};
use std::time::SystemTime;

/// Convert SystemTime to Unix timestamp in seconds
pub fn system_time_to_timestamp(time: SystemTime) -> i64 {
    time.duration_since(SystemTime::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

/// Map domain ProcessState to protobuf ProcessState
pub fn domain_state_to_proto(state: DomainProcessState) -> i32 {
    match state {
        DomainProcessState::Created => ProtoProcessState::Created as i32,
        DomainProcessState::Stopped => ProtoProcessState::Stopped as i32,
        DomainProcessState::Starting => ProtoProcessState::Starting as i32,
        DomainProcessState::Running => ProtoProcessState::Running as i32,
        DomainProcessState::Stopping => ProtoProcessState::Stopping as i32,
        DomainProcessState::Exited => ProtoProcessState::Exited as i32,
        DomainProcessState::Failed => ProtoProcessState::Crashed as i32, // Map Failed to Crashed for proto
        DomainProcessState::Restarting => ProtoProcessState::Starting as i32, // Map Restarting to Starting
    }
}

/// Map domain Process entity to protobuf Process message
pub fn domain_process_to_proto(process: &Process) -> ProtoProcess {
    ProtoProcess {
        id: process.id().to_string(),
        name: process.name().to_string(),
        pid: process.pid().unwrap_or(0),
        command: process.command().to_string(),
        args: process.args().to_vec(),
        state: domain_state_to_proto(process.state()),
        run_count: process.run_count(),
        exit_code: process.exit_code().unwrap_or(0),
        signal: String::new(),
        created_at: system_time_to_timestamp(process.created_at()),
        started_at: process
            .started_at()
            .map(system_time_to_timestamp)
            .unwrap_or(0),
        ended_at: process
            .stopped_at()
            .map(system_time_to_timestamp)
            .unwrap_or(0),
    }
}

/// Parse UUID string from proto ID
pub fn parse_process_id(id: &str) -> Result<ProcessId, String> {
    uuid::Uuid::parse_str(id)
        .map(ProcessId::from)
        .map_err(|e| format!("Invalid process ID: {}", e))
}

/// Map domain KillMode to protobuf KillMode
pub fn map_kill_mode_to_proto(kill_mode: DomainKillMode) -> i32 {
    match kill_mode {
        DomainKillMode::ControlGroup => ProtoKillMode::ControlGroup as i32,
        DomainKillMode::ProcessGroup => ProtoKillMode::ProcessGroup as i32,
        DomainKillMode::Process => ProtoKillMode::Process as i32,
        DomainKillMode::Mixed => ProtoKillMode::Mixed as i32,
    }
}

/// Map domain HealthCheckType to protobuf HealthCheckType
fn map_health_check_type_to_proto(hc_type: HealthCheckType) -> i32 {
    match hc_type {
        HealthCheckType::Http => ProtoHealthCheckType::Http as i32,
        HealthCheckType::Tcp => ProtoHealthCheckType::Tcp as i32,
        HealthCheckType::Exec => ProtoHealthCheckType::Exec as i32,
    }
}

/// Map domain HealthCheck to protobuf HealthCheck
pub fn map_health_check_to_proto(hc: &HealthCheck) -> ProtoHealthCheck {
    ProtoHealthCheck {
        r#type: map_health_check_type_to_proto(hc.check_type),
        interval: hc.interval,
        timeout: hc.timeout,
        retries: hc.retries,
        start_period: hc.start_period,
        restart_after: hc.restart_after,
        http_endpoint: hc.http_endpoint.clone().unwrap_or_default(),
        http_method: hc.http_method.clone().unwrap_or_default(),
        http_expected_status: hc.http_expected_status.unwrap_or(0) as u32,
        tcp_host: hc.tcp_host.clone().unwrap_or_default(),
        tcp_port: hc.tcp_port.unwrap_or(0) as u32,
        exec_command: hc.exec_command.clone().unwrap_or_default(),
        exec_args: hc.exec_args.clone().unwrap_or_default(),
    }
}

/// Map domain ResourceLimits to protobuf ResourceLimits
pub fn map_resource_limits_to_proto(limits: &ResourceLimits) -> ProtoResourceLimits {
    ProtoResourceLimits {
        cpu_request: 0, // Not supported yet
        cpu_limit: limits.cpu_millis.unwrap_or(0),
        memory_request: 0, // Not supported yet
        memory_limit: limits.memory_bytes.unwrap_or(0),
        pids_limit: limits.max_pids.unwrap_or(0),
        oom_score_adj: 0, // Not supported yet
    }
}

// ============================================================================
// REVERSE MAPPERS: Proto -> Domain (for incoming requests)
// ============================================================================

/// Map protobuf RestartPolicy to domain RestartPolicy
pub fn proto_to_restart_policy(proto_policy: i32) -> Option<DomainRestartPolicy> {
    ProtoRestartPolicy::from_i32(proto_policy).map(|p| match p {
        ProtoRestartPolicy::Never => DomainRestartPolicy::Never,
        ProtoRestartPolicy::Always => DomainRestartPolicy::Always,
        ProtoRestartPolicy::OnFailure => DomainRestartPolicy::OnFailure,
        ProtoRestartPolicy::OnSuccess => DomainRestartPolicy::OnSuccess,
    })
}

/// Map protobuf ProcessType to domain ProcessType
pub fn proto_to_process_type(proto_type: i32) -> Option<DomainProcessType> {
    ProtoProcessType::from_i32(proto_type).map(|p| match p {
        ProtoProcessType::Simple => DomainProcessType::Simple,
        ProtoProcessType::Forking => DomainProcessType::Forking,
        ProtoProcessType::Oneshot => DomainProcessType::Oneshot,
        ProtoProcessType::Notify => DomainProcessType::Notify,
    })
}

/// Map protobuf KillMode to domain KillMode
pub fn proto_to_kill_mode(proto_mode: i32) -> Option<DomainKillMode> {
    ProtoKillMode::from_i32(proto_mode).map(|m| match m {
        ProtoKillMode::ControlGroup => DomainKillMode::ControlGroup,
        ProtoKillMode::ProcessGroup => DomainKillMode::ProcessGroup,
        ProtoKillMode::Process => DomainKillMode::Process,
        ProtoKillMode::Mixed => DomainKillMode::Mixed,
    })
}

/// Map protobuf HealthCheckType to domain HealthCheckType
fn proto_to_health_check_type(proto_type: i32) -> Option<HealthCheckType> {
    ProtoHealthCheckType::from_i32(proto_type).map(|t| match t {
        ProtoHealthCheckType::Http => HealthCheckType::Http,
        ProtoHealthCheckType::Tcp => HealthCheckType::Tcp,
        ProtoHealthCheckType::Exec => HealthCheckType::Exec,
    })
}

/// Map protobuf HealthCheck to domain HealthCheck
pub fn proto_to_health_check(proto_hc: &ProtoHealthCheck) -> Option<HealthCheck> {
    let check_type = proto_to_health_check_type(proto_hc.r#type)?;

    Some(HealthCheck {
        check_type,
        interval: proto_hc.interval,
        timeout: proto_hc.timeout,
        retries: proto_hc.retries,
        start_period: proto_hc.start_period,
        restart_after: proto_hc.restart_after,
        http_endpoint: if proto_hc.http_endpoint.is_empty() {
            None
        } else {
            Some(proto_hc.http_endpoint.clone())
        },
        http_method: if proto_hc.http_method.is_empty() {
            None
        } else {
            Some(proto_hc.http_method.clone())
        },
        http_expected_status: if proto_hc.http_expected_status == 0 {
            None
        } else {
            Some(proto_hc.http_expected_status as u16)
        },
        tcp_host: if proto_hc.tcp_host.is_empty() {
            None
        } else {
            Some(proto_hc.tcp_host.clone())
        },
        tcp_port: if proto_hc.tcp_port == 0 {
            None
        } else {
            Some(proto_hc.tcp_port as u16)
        },
        exec_command: if proto_hc.exec_command.is_empty() {
            None
        } else {
            Some(proto_hc.exec_command.clone())
        },
        exec_args: if proto_hc.exec_args.is_empty() {
            None
        } else {
            Some(proto_hc.exec_args.clone())
        },
    })
}

/// Map protobuf ResourceLimits to domain ResourceLimits
pub fn proto_to_resource_limits(proto_limits: &ProtoResourceLimits) -> ResourceLimits {
    ResourceLimits {
        cpu_millis: if proto_limits.cpu_limit > 0 {
            Some(proto_limits.cpu_limit)
        } else {
            None
        },
        memory_bytes: if proto_limits.memory_limit > 0 {
            Some(proto_limits.memory_limit)
        } else {
            None
        },
        max_pids: if proto_limits.pids_limit > 0 {
            Some(proto_limits.pids_limit)
        } else {
            None
        },
    }
}

/// Parse PathCondition from string (e.g., "/path", "!/path", "|/path")
pub fn parse_path_conditions(paths: &[String]) -> Vec<PathCondition> {
    paths.iter().map(|s| PathCondition::parse(s)).collect()
}

/// Parse restart policy from string
pub fn parse_restart_policy_string(s: &str) -> Option<DomainRestartPolicy> {
    match s.to_lowercase().as_str() {
        "never" => Some(DomainRestartPolicy::Never),
        "always" => Some(DomainRestartPolicy::Always),
        "on-failure" => Some(DomainRestartPolicy::OnFailure),
        "on-success" => Some(DomainRestartPolicy::OnSuccess),
        _ => None,
    }
}

/// Parse kill mode from string
pub fn parse_kill_mode_string(s: &str) -> Option<DomainKillMode> {
    match s.to_lowercase().as_str() {
        "control-group" => Some(DomainKillMode::ControlGroup),
        "process-group" => Some(DomainKillMode::ProcessGroup),
        "process" => Some(DomainKillMode::Process),
        "mixed" => Some(DomainKillMode::Mixed),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_domain_state_to_proto() {
        assert_eq!(
            domain_state_to_proto(DomainProcessState::Created),
            ProtoProcessState::Created as i32
        );
        assert_eq!(
            domain_state_to_proto(DomainProcessState::Running),
            ProtoProcessState::Running as i32
        );
        assert_eq!(
            domain_state_to_proto(DomainProcessState::Failed),
            ProtoProcessState::Crashed as i32
        );
    }

    #[test]
    fn test_domain_process_to_proto() {
        let process = Process::builder("test".to_string(), "/bin/test".to_string())
            .build()
            .unwrap();
        let proto = domain_process_to_proto(&process);

        assert_eq!(proto.name, "test");
        assert_eq!(proto.command, "/bin/test");
        assert_eq!(proto.state, ProtoProcessState::Created as i32);
    }

    #[test]
    fn test_parse_process_id_valid() {
        let uuid_str = "550e8400-e29b-41d4-a716-446655440000";
        let result = parse_process_id(uuid_str);
        assert!(result.is_ok());
    }

    #[test]
    fn test_parse_process_id_invalid() {
        let result = parse_process_id("not-a-uuid");
        assert!(result.is_err());
    }
}
