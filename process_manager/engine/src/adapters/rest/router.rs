//! REST API router configuration

use super::handlers::{
    create_process, delete_process, get_process, list_processes, restart_process, start_process,
    stop_process, AppState,
};
use axum::{
    routing::{delete, get, post},
    Router,
};

/// Build the REST API router
pub fn build_router(state: AppState) -> Router {
    Router::new()
        // Process CRUD
        .route("/processes", post(create_process))
        .route("/processes", get(list_processes))
        .route("/processes/:id", get(get_process))
        .route("/processes/:id", delete(delete_process))
        // Process lifecycle operations
        .route("/processes/:id/start", post(start_process))
        .route("/processes/:id/stop", post(stop_process))
        .route("/processes/:id/restart", post(restart_process))
        .with_state(state)
}
