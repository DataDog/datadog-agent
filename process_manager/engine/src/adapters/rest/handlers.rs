//! REST API handlers using axum

use crate::application::Application;
use crate::domain::{
    CreateProcessCommand, DeleteProcessCommand, GetProcessStatusQuery, RestartProcessCommand,
    StartProcessCommand, StopProcessCommand,
};
use axum::{
    extract::{Path, State},
    http::StatusCode,
    response::{IntoResponse, Response},
    Json,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tracing::{debug, error, info};

/// Shared application state
pub type AppState = Arc<Application>;

/// Error response
#[derive(Serialize)]
pub struct ErrorResponse {
    pub error: String,
}

impl IntoResponse for ErrorResponse {
    fn into_response(self) -> Response {
        (StatusCode::INTERNAL_SERVER_ERROR, Json(self)).into_response()
    }
}

/// Create process request
#[derive(Deserialize)]
pub struct CreateProcessRequest {
    pub name: String,
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
}

/// Create process response
#[derive(Serialize)]
pub struct CreateProcessResponse {
    pub id: String,
    pub name: String,
}

/// Process info response (for list view)
#[derive(Serialize)]
pub struct ProcessInfo {
    pub id: String,
    pub name: String,
    pub command: String,
    pub state: String,
    pub pid: Option<u32>,
}

/// Process detail response (for get by id)
#[derive(Serialize)]
pub struct ProcessDetailResponse {
    pub id: String,
    pub name: String,
    pub command: String,
    pub state: String,
    pub pid: Option<u32>,
    pub restart_policy: String,
    // Environment and runtime context
    pub working_dir: Option<String>,
    #[serde(skip_serializing_if = "std::collections::HashMap::is_empty")]
    pub env: std::collections::HashMap<String, String>,
    pub environment_file: Option<String>,
    pub pidfile: Option<String>,
    pub stdout: Option<String>,
    pub stderr: Option<String>,
    pub user: Option<String>,
    pub group: Option<String>,
    // Lifecycle and exit behavior
    pub success_exit_status: Vec<i32>,
    pub exec_start_pre: Vec<String>,
    pub exec_start_post: Vec<String>,
    pub exec_stop_post: Vec<String>,
    // Dependencies
    pub requires: Vec<String>,
    pub binds_to: Vec<String>,
    pub conflicts: Vec<String>,
    pub after: Vec<String>,
    pub before: Vec<String>,
    pub wants: Vec<String>,
    // Health check
    pub health_status: String,
    pub health_check_failures: u32,
    // Resource limits
    pub resource_limits: ResourceLimitsResponse,
}

/// Resource limits response
#[derive(Serialize)]
pub struct ResourceLimitsResponse {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cpu_millis: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub memory_bytes: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_pids: Option<u32>,
}

/// List processes response
#[derive(Serialize)]
pub struct ListProcessesResponse {
    pub processes: Vec<ProcessInfo>,
}

/// Simple success response
#[derive(Serialize)]
pub struct SuccessResponse {
    pub message: String,
}

// ===== Handlers =====

/// POST /processes - Create a new process
pub async fn create_process(
    State(registry): State<AppState>,
    Json(req): Json<CreateProcessRequest>,
) -> Result<Json<CreateProcessResponse>, (StatusCode, Json<ErrorResponse>)> {
    info!(name = %req.name, command = %req.command, args = ?req.args, "REST Create request");

    let command = CreateProcessCommand {
        name: req.name,
        command: req.command,
        args: req.args,
        ..Default::default()
    };

    let result = registry
        .create_process()
        .execute(command)
        .await
        .map_err(|e| {
            error!(error = %e, "Create process failed");
            (
                StatusCode::BAD_REQUEST,
                Json(ErrorResponse {
                    error: e.to_string(),
                }),
            )
        })?;

    debug!(process_id = %result.id, "Process created");

    Ok(Json(CreateProcessResponse {
        id: result.id.to_string(),
        name: result.name,
    }))
}

/// GET /processes - List all processes
pub async fn list_processes(
    State(registry): State<AppState>,
) -> Result<Json<ListProcessesResponse>, (StatusCode, Json<ErrorResponse>)> {
    info!("REST List request");

    let result = registry.list_processes().execute().await.map_err(|e| {
        error!(error = %e, "List processes failed");
        (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(ErrorResponse {
                error: e.to_string(),
            }),
        )
    })?;

    let processes = result
        .processes
        .iter()
        .map(|p| ProcessInfo {
            id: p.id().to_string(),
            name: p.name().to_string(),
            command: p.command().to_string(),
            state: format!("{:?}", p.state()),
            pid: p.pid(),
        })
        .collect();

    debug!(count = result.processes.len(), "Processes listed");

    Ok(Json(ListProcessesResponse { processes }))
}

/// GET /processes/:id - Get process status
pub async fn get_process(
    State(registry): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<ProcessDetailResponse>, (StatusCode, Json<ErrorResponse>)> {
    info!(process_id = %id, "REST Get process request");

    // Try to parse as UUID first, otherwise treat as name
    let query = if let Ok(uuid) = uuid::Uuid::parse_str(&id) {
        GetProcessStatusQuery::from_id(crate::domain::ProcessId::from(uuid))
    } else {
        GetProcessStatusQuery::from_name(id.clone())
    };

    let result = registry
        .get_process_status()
        .execute(query)
        .await
        .map_err(|e| {
            error!(error = %e, "Get process failed");
            (
                StatusCode::NOT_FOUND,
                Json(ErrorResponse {
                    error: format!("Process not found: {}", e),
                }),
            )
        })?;

    debug!(process_id = %id, "Process retrieved");

    Ok(Json(ProcessDetailResponse {
        id: result.process.id().to_string(),
        name: result.process.name().to_string(),
        command: result.process.command().to_string(),
        state: format!("{:?}", result.process.state()),
        pid: result.process.pid(),
        restart_policy: format!("{:?}", result.process.restart_policy()),
        // Environment and runtime context
        working_dir: result.process.working_dir().map(|s| s.to_string()),
        env: result.process.env().clone(),
        environment_file: result.process.environment_file().map(|s| s.to_string()),
        pidfile: result.process.pidfile().map(|s| s.to_string()),
        stdout: result.process.stdout().map(|s| s.to_string()),
        stderr: result.process.stderr().map(|s| s.to_string()),
        user: result.process.user().map(|s| s.to_string()),
        group: result.process.group().map(|s| s.to_string()),
        // Lifecycle and exit behavior
        success_exit_status: result.process.success_exit_status().to_vec(),
        exec_start_pre: result.process.exec_start_pre().to_vec(),
        exec_start_post: result.process.exec_start_post().to_vec(),
        exec_stop_post: result.process.exec_stop_post().to_vec(),
        // Dependencies
        requires: result.process.requires().to_vec(),
        binds_to: result.process.binds_to().to_vec(),
        conflicts: result.process.conflicts().to_vec(),
        after: result.process.after().to_vec(),
        before: result.process.before().to_vec(),
        wants: result.process.wants().to_vec(),
        // Health check
        health_status: result.process.health_status().to_string(),
        health_check_failures: result.process.health_check_failures(),
        // Resource limits
        resource_limits: ResourceLimitsResponse {
            cpu_millis: result.process.resource_limits().cpu_millis,
            memory_bytes: result.process.resource_limits().memory_bytes,
            max_pids: result.process.resource_limits().max_pids,
        },
    }))
}

/// POST /processes/:id/start - Start a process
pub async fn start_process(
    State(registry): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<SuccessResponse>, (StatusCode, Json<ErrorResponse>)> {
    info!(process_id = %id, "REST Start request");

    // Try to parse as UUID first, otherwise treat as name
    let command = if let Ok(uuid) = uuid::Uuid::parse_str(&id) {
        StartProcessCommand::from_id(crate::domain::ProcessId::from(uuid))
    } else {
        StartProcessCommand::from_name(id.clone())
    };

    registry
        .start_process()
        .execute(command)
        .await
        .map_err(|e| {
            error!(error = %e, "Start process failed");
            (
                StatusCode::BAD_REQUEST,
                Json(ErrorResponse {
                    error: e.to_string(),
                }),
            )
        })?;

    debug!(process_id = %id, "Process started");

    Ok(Json(SuccessResponse {
        message: "Process started successfully".to_string(),
    }))
}

/// POST /processes/:id/stop - Stop a process
pub async fn stop_process(
    State(registry): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<SuccessResponse>, (StatusCode, Json<ErrorResponse>)> {
    info!(process_id = %id, "REST Stop request");

    // Try to parse as UUID first, otherwise treat as name
    let command = if let Ok(uuid) = uuid::Uuid::parse_str(&id) {
        StopProcessCommand::from_id(crate::domain::ProcessId::from(uuid))
    } else {
        StopProcessCommand::from_name(id.clone())
    };

    registry
        .stop_process()
        .execute(command)
        .await
        .map_err(|e| {
            error!(error = %e, "Stop process failed");
            (
                StatusCode::BAD_REQUEST,
                Json(ErrorResponse {
                    error: e.to_string(),
                }),
            )
        })?;

    debug!(process_id = %id, "Process stopped");

    Ok(Json(SuccessResponse {
        message: "Process stopped successfully".to_string(),
    }))
}

/// POST /processes/:id/restart - Restart a process
pub async fn restart_process(
    State(registry): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<SuccessResponse>, (StatusCode, Json<ErrorResponse>)> {
    info!(process_id = %id, "REST Restart request");

    // Try to parse as UUID first, otherwise treat as name
    let command = if let Ok(uuid) = uuid::Uuid::parse_str(&id) {
        RestartProcessCommand::from_id(crate::domain::ProcessId::from(uuid))
    } else {
        RestartProcessCommand::from_name(id.clone())
    };

    registry
        .restart_process()
        .execute(command)
        .await
        .map_err(|e| {
            error!(error = %e, "Restart process failed");
            (
                StatusCode::BAD_REQUEST,
                Json(ErrorResponse {
                    error: e.to_string(),
                }),
            )
        })?;

    debug!(process_id = %id, "Process restarted");

    Ok(Json(SuccessResponse {
        message: "Process restarted successfully".to_string(),
    }))
}

/// DELETE /processes/:id - Delete a process
pub async fn delete_process(
    State(registry): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<SuccessResponse>, (StatusCode, Json<ErrorResponse>)> {
    info!(process_id = %id, "REST Delete request");

    // Try to parse as UUID first, otherwise treat as name
    // TODO: Support force flag via query parameter
    let command = if let Ok(uuid) = uuid::Uuid::parse_str(&id) {
        DeleteProcessCommand::from_id(crate::domain::ProcessId::from(uuid), false)
    } else {
        DeleteProcessCommand::from_name(id.clone(), false)
    };

    registry
        .delete_process()
        .execute(command)
        .await
        .map_err(|e| {
            error!(error = %e, "Delete process failed");
            (
                StatusCode::PRECONDITION_FAILED,
                Json(ErrorResponse {
                    error: e.to_string(),
                }),
            )
        })?;

    debug!(process_id = %id, "Process deleted");

    Ok(Json(SuccessResponse {
        message: "Process deleted successfully".to_string(),
    }))
}
