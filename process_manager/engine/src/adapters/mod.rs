//! Driving Adapters Layer
//!
//! This module contains the "driving" or "primary" adapters.
//! These adapters drive the application by accepting external requests and translating
//! them into domain commands/queries.
//!
//! ## Available Adapters
//!
//! - **gRPC**: Protocol Buffers-based remote procedure calls
//! - **REST**: RESTful HTTP API with JSON
//!
//! ## Usage
//!
//! ```rust,no_run
//! use pm_engine::application::Application;
//! use pm_engine::infrastructure::{InMemoryProcessRepository, TokioProcessExecutor};
//! use pm_engine::adapters::{grpc::ProcessManagerService, rest::build_router};
//! use std::sync::Arc;
//!
//! # async fn example() {
//! // Setup infrastructure
//! let repository = Arc::new(InMemoryProcessRepository::new());
//! let executor = Arc::new(TokioProcessExecutor::new());
//! let registry = Arc::new(Application::new(repository, executor));
//!
//! // Setup gRPC adapter
//! let grpc_service = ProcessManagerService::new(registry.clone());
//!
//! // Setup REST adapter
//! let rest_router = build_router(registry);
//! # }
//! ```

pub mod grpc;
pub mod rest;
