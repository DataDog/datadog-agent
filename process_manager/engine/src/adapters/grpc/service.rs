//! gRPC ProcessManager service implementation
//! Driving adapter that exposes use cases through gRPC

use crate::application::Application;
use crate::domain::{
    CreateProcessCommand, DeleteProcessCommand, GetProcessStatusQuery, StartProcessCommand,
    StopProcessCommand,
};
use crate::proto::process_manager::{
    process_manager_server::ProcessManager, CreateRequest, CreateResponse, DeleteRequest,
    DeleteResponse, DescribeRequest, DescribeResponse, GetResourceUsageRequest,
    GetResourceUsageResponse, ListRequest, ListResponse, ReloadConfigRequest, ReloadConfigResponse,
    ResourceUsageInfo, StartRequest, StartResponse, StopRequest, StopResponse, UpdateRequest,
    UpdateResponse,
};
use std::sync::Arc;
use tonic::{Request, Response, Status};
use tracing::{debug, error, info};

use super::mappers::{
    domain_process_to_proto, domain_state_to_proto, map_health_check_to_proto,
    map_kill_mode_to_proto, map_resource_limits_to_proto, parse_process_id,
    system_time_to_timestamp,
};

/// gRPC service implementation
pub struct ProcessManagerService {
    registry: Arc<Application>,
}

impl ProcessManagerService {
    pub fn new(registry: Arc<Application>) -> Self {
        Self { registry }
    }
}

#[tonic::async_trait]
impl ProcessManager for ProcessManagerService {
    async fn create(
        &self,
        request: Request<CreateRequest>,
    ) -> Result<Response<CreateResponse>, Status> {
        let req = request.into_inner();

        info!(
            name = %req.name,
            command = %req.command,
            args = ?req.args,
            "gRPC Create request received"
        );

        // Save auto_start flag for later (before consuming req)
        let auto_start = req.auto_start;

        // Convert proto request to domain command using TryFrom
        let command = CreateProcessCommand::try_from(req)?;

        // Execute use case
        let result = self
            .registry
            .create_process()
            .execute(command)
            .await
            .map_err(|e| {
                error!(error = %e, "Create process failed");
                Status::internal(format!("Failed to create process: {}", e))
            })?;

        debug!(
            process_id = %result.id,
            name = %result.name,
            auto_start = auto_start,
            "Process created successfully"
        );

        // Auto-start if requested
        let final_state = if auto_start {
            info!(
                process_id = %result.id,
                name = %result.name,
                "Auto-starting process"
            );

            // StartProcess use case handles recursive dependency resolution
            let start_command = StartProcessCommand::from_id(result.id);
            match self.registry.start_process().execute(start_command).await {
                Ok(_) => {
                    info!(process_id = %result.id, "Process auto-started successfully");
                    crate::domain::ProcessState::Running
                }
                Err(e) => {
                    error!(process_id = %result.id, error = %e, "Failed to auto-start process");
                    // Process was created but failed to start
                    crate::domain::ProcessState::Created
                }
            }
        } else {
            crate::domain::ProcessState::Created
        };

        Ok(Response::new(CreateResponse {
            id: result.id.to_string(),
            state: domain_state_to_proto(final_state),
        }))
    }

    async fn start(
        &self,
        request: Request<StartRequest>,
    ) -> Result<Response<StartResponse>, Status> {
        let req = request.into_inner();

        info!(process_id_or_name = %req.id, "gRPC Start request received");

        // Try to parse as UUID first, otherwise treat as name
        let command = if let Ok(process_id) = parse_process_id(&req.id) {
            StartProcessCommand::from_id(process_id)
        } else {
            StartProcessCommand::from_name(req.id.clone())
        };

        // Execute use case
        let _result = self
            .registry
            .start_process()
            .execute(command)
            .await
            .map_err(|e| {
                error!(error = %e, "Start process failed");
                Status::internal(format!("Failed to start process: {}", e))
            })?;

        debug!(process_id = %req.id, "Process started successfully");

        Ok(Response::new(StartResponse {
            state: domain_state_to_proto(crate::domain::ProcessState::Running),
        }))
    }

    async fn stop(&self, request: Request<StopRequest>) -> Result<Response<StopResponse>, Status> {
        let req = request.into_inner();

        info!(process_id_or_name = %req.id, "gRPC Stop request received");

        // Try to parse as UUID first, otherwise treat as name
        let command = if let Ok(process_id) = parse_process_id(&req.id) {
            StopProcessCommand::from_id(process_id)
        } else {
            StopProcessCommand::from_name(req.id.clone())
        };

        // Execute use case
        let _result = self
            .registry
            .stop_process()
            .execute(command)
            .await
            .map_err(|e| {
                error!(error = %e, "Stop process failed");
                Status::internal(format!("Failed to stop process: {}", e))
            })?;

        debug!(process_id = %req.id, "Process stopped successfully");

        Ok(Response::new(StopResponse {
            state: domain_state_to_proto(crate::domain::ProcessState::Stopped),
        }))
    }

    async fn list(&self, _request: Request<ListRequest>) -> Result<Response<ListResponse>, Status> {
        info!("gRPC List request received");

        // Execute use case
        let result = self
            .registry
            .list_processes()
            .execute()
            .await
            .map_err(|e| {
                error!(error = %e, "List processes failed");
                Status::internal(format!("Failed to list processes: {}", e))
            })?;

        // Map to proto
        let processes = result
            .processes
            .iter()
            .map(domain_process_to_proto)
            .collect();

        debug!(
            count = result.processes.len(),
            "Processes listed successfully"
        );

        Ok(Response::new(ListResponse { processes }))
    }

    async fn describe(
        &self,
        request: Request<DescribeRequest>,
    ) -> Result<Response<DescribeResponse>, Status> {
        let req = request.into_inner();

        info!(process_id_or_name = %req.id, "gRPC Describe request received");

        // Try to parse as UUID first, otherwise treat as name
        let query = if let Ok(process_id) = parse_process_id(&req.id) {
            GetProcessStatusQuery::from_id(process_id)
        } else {
            GetProcessStatusQuery::from_name(req.id.clone())
        };

        // Execute use case
        let result = self
            .registry
            .get_process_status()
            .execute(query)
            .await
            .map_err(|e| {
                error!(error = %e, "Get process status failed");
                Status::not_found(format!("Process not found: {}", e))
            })?;

        // Map to proto detail
        let detail = crate::proto::process_manager::ProcessDetail {
            id: result.process.id().to_string(),
            name: result.process.name().to_string(),
            description: result.process.description().unwrap_or("").to_string(),
            pid: result.process.pid().unwrap_or(0),
            command: result.process.command().to_string(),
            args: result.process.args().to_vec(),
            state: domain_state_to_proto(result.process.state()),
            restart_policy: result.process.restart_policy().to_string(),
            run_count: result.process.run_count(),
            exit_code: result.process.exit_code().unwrap_or(0),
            signal: String::new(),
            created_at: system_time_to_timestamp(result.process.created_at()),
            started_at: result
                .process
                .started_at()
                .map(system_time_to_timestamp)
                .unwrap_or(0),
            ended_at: result
                .process
                .stopped_at()
                .map(system_time_to_timestamp)
                .unwrap_or(0),
            working_dir: result.process.working_dir().unwrap_or("").to_string(),
            env: result.process.env().clone(),
            // Restart timing and failure tracking
            restart_sec: result.process.restart_sec(),
            restart_max_delay: result.process.restart_max_delay_sec(),
            consecutive_failures: result.process.consecutive_failures(),
            // Start limit protection
            start_limit_burst: result.process.start_limit_burst(),
            start_limit_interval: result.process.start_limit_interval_sec(),
            // Timeouts
            timeout_start_sec: result.process.timeout_start_sec(),
            timeout_stop_sec: result.process.timeout_stop_sec(),
            // Kill configuration
            kill_signal_str: result.process.kill_signal().to_string(),
            kill_mode_val: map_kill_mode_to_proto(result.process.kill_mode()),
            // Runtime context
            environment_file: result.process.environment_file().unwrap_or("").to_string(),
            pidfile: result.process.pidfile().unwrap_or("").to_string(),
            stdout: result.process.stdout().unwrap_or("").to_string(),
            stderr: result.process.stderr().unwrap_or("").to_string(),
            user: result.process.user().unwrap_or("").to_string(),
            group: result.process.group().unwrap_or("").to_string(),
            // Lifecycle and exit behavior
            success_exit_status: result.process.success_exit_status().to_vec(),
            exec_start_pre: result.process.exec_start_pre().to_vec(),
            exec_start_post: result.process.exec_start_post().to_vec(),
            exec_stop_post: result.process.exec_stop_post().to_vec(),
            // Dependencies from process entity
            after: result.process.after().to_vec(),
            before: result.process.before().to_vec(),
            requires: result.process.requires().to_vec(),
            wants: result.process.wants().to_vec(),
            binds_to: result.process.binds_to().to_vec(),
            conflicts: result.process.conflicts().to_vec(),
            process_type: crate::proto::process_manager::ProcessType::Simple as i32,
            // Health check configuration and status
            health_check: result.process.health_check().map(map_health_check_to_proto),
            health_status: result.process.health_status().to_string(),
            health_check_failures: result.process.health_check_failures(),
            last_health_check: result
                .process
                .last_health_check()
                .map(system_time_to_timestamp)
                .unwrap_or(0),
            resource_limits: Some(map_resource_limits_to_proto(
                result.process.resource_limits(),
            )),
            condition_path_exists: result
                .process
                .condition_path_exists()
                .iter()
                .map(|c| c.to_string())
                .collect(),
            runtime_directory: result.process.runtime_directory().to_vec(),
            ambient_capabilities: result.process.ambient_capabilities().to_vec(),
        };

        debug!(process_id = %req.id, "Process described successfully");

        Ok(Response::new(DescribeResponse {
            detail: Some(detail),
        }))
    }

    async fn delete(
        &self,
        request: Request<DeleteRequest>,
    ) -> Result<Response<DeleteResponse>, Status> {
        let req = request.into_inner();

        info!(process_id_or_name = %req.id, force = req.force, "gRPC Delete request received");

        // Try to parse as UUID first, otherwise treat as name
        let command = if let Ok(process_id) = parse_process_id(&req.id) {
            DeleteProcessCommand::from_id(process_id, req.force)
        } else {
            DeleteProcessCommand::from_name(req.id.clone(), req.force)
        };

        // Execute use case
        let _result = self
            .registry
            .delete_process()
            .execute(command)
            .await
            .map_err(|e| {
                error!(error = %e, "Delete process failed");
                Status::failed_precondition(format!("Failed to delete process: {}", e))
            })?;

        debug!(process_id = %req.id, "Process deleted successfully");

        Ok(Response::new(DeleteResponse {}))
    }

    async fn update(
        &self,
        request: Request<UpdateRequest>,
    ) -> Result<Response<UpdateResponse>, Status> {
        let req = request.into_inner();

        info!(process_id_or_name = %req.id, dry_run = req.dry_run, "gRPC Update request received");

        // Convert proto request to domain command using TryFrom
        let command = crate::domain::UpdateProcessCommand::try_from(req)?;

        // Execute use case
        let result = self
            .registry
            .update_process()
            .execute(command)
            .await
            .map_err(|e| {
                error!(error = %e, "Update process failed");
                Status::internal(format!("Failed to update process: {}", e))
            })?;

        debug!(
            process_id = %result.process_id,
            updated_fields = ?result.updated_fields,
            restart_required = ?result.restart_required_fields,
            restarted = result.process_restarted,
            "Process updated successfully"
        );

        Ok(Response::new(UpdateResponse {
            updated_fields: result.updated_fields,
            restart_required_fields: result.restart_required_fields,
            process_restarted: result.process_restarted,
        }))
    }

    async fn reload_config(
        &self,
        _request: Request<ReloadConfigRequest>,
    ) -> Result<Response<ReloadConfigResponse>, Status> {
        // Not implemented yet - return unimplemented
        Err(Status::unimplemented("ReloadConfig not yet implemented"))
    }

    async fn get_resource_usage(
        &self,
        request: Request<GetResourceUsageRequest>,
    ) -> Result<Response<GetResourceUsageResponse>, Status> {
        let req = request.into_inner();

        // Parse ID or name
        let command = if let Ok(uuid) = uuid::Uuid::parse_str(&req.id) {
            crate::domain::GetResourceUsageCommand::from_id(crate::domain::ProcessId::from(uuid))
        } else {
            crate::domain::GetResourceUsageCommand::from_name(req.id.clone())
        };

        // Execute use case
        let result = self
            .registry
            .get_resource_usage()
            .execute(command)
            .await
            .map_err(|e| {
                if matches!(e, crate::domain::DomainError::ProcessNotFound(_)) {
                    Status::not_found(format!("Process '{}' not found", req.id))
                } else {
                    Status::internal(format!("Failed to get resource usage: {}", e))
                }
            })?;

        // Map to protobuf response
        let usage_info = ResourceUsageInfo {
            memory_current: result.usage.memory_current.unwrap_or(0),
            memory_limit: result.limits.memory_bytes.unwrap_or(0),
            cpu_usage_usec: result.usage.cpu_usage_usec.unwrap_or(0),
            cpu_user_usec: result.usage.cpu_user_usec.unwrap_or(0),
            cpu_system_usec: result.usage.cpu_system_usec.unwrap_or(0),
            pids_current: result.usage.pids_current.unwrap_or(0),
            pids_limit: result.limits.max_pids.unwrap_or(0),
        };

        Ok(Response::new(GetResourceUsageResponse {
            usage: Some(usage_info),
        }))
    }

    async fn get_status(
        &self,
        _request: Request<crate::proto::process_manager::GetStatusRequest>,
    ) -> Result<Response<crate::proto::process_manager::GetStatusResponse>, Status> {
        debug!("gRPC GetStatus request received");

        // Get all processes to calculate statistics
        let list_result = self
            .registry
            .list_processes()
            .execute()
            .await
            .map_err(|e| Status::internal(format!("Failed to list processes: {}", e)))?;

        let total_processes = list_result.processes.len() as u32;
        let running_processes = list_result
            .processes
            .iter()
            .filter(|p| p.is_running())
            .count() as u32;
        let stopped_processes = list_result
            .processes
            .iter()
            .filter(|p| p.state().to_string() == "stopped")
            .count() as u32;
        let failed_processes = list_result
            .processes
            .iter()
            .filter(|p| {
                let state = p.state().to_string();
                state == "failed" || state == "crashed"
            })
            .count() as u32;

        // Calculate uptime
        // Note: We'll need to pass startup_time from daemon, for now use a placeholder
        let uptime_seconds = 0; // TODO: Pass startup_time from daemon

        let response = crate::proto::process_manager::GetStatusResponse {
            ready: true,
            version: env!("CARGO_PKG_VERSION").to_string(),
            uptime_seconds,
            total_processes,
            running_processes,
            stopped_processes,
            failed_processes,
            supervisor_healthy: true,   // TODO: Add actual health check
            repository_healthy: true,   // TODO: Add actual health check
            config_path: String::new(), // TODO: Pass config path from daemon
        };

        Ok(Response::new(response))
    }
}
