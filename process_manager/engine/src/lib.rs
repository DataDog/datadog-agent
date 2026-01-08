//! Process Manager Engine
//!
//! A library for managing system processes with support for:
//! - Process lifecycle management (create, start, stop)
//! - State tracking and monitoring
//! - Automatic restart policies (systemd-style)
//! - gRPC and REST APIs for cross-language integration
//!
//! ## Architecture
//!
//! This engine follows hexagonal (ports and adapters) architecture:
//!
//! - **Domain**: Core business logic, entities, and use cases
//! - **Application**: Use case orchestration and registry
//! - **Adapters**: gRPC and REST API implementations
//! - **Infrastructure**: Concrete implementations (repository, executor)
//!
//! ## Usage
//!
//! The daemon binary (`dd-procmgrd`) uses these modules directly:
//!
//! ```rust,ignore
//! use pm_engine::{
//!     adapters::grpc::ProcessManagerService,
//!     application::Application,
//!     domain::services::ProcessSupervisionService,
//!     infrastructure::{InMemoryProcessRepository, TokioProcessExecutor},
//! };
//! ```

// Module declarations
pub mod constants; // Used by E2E tests for port allocation

// Core architecture modules (hexagonal architecture)
pub mod adapters;
pub mod application;
pub mod domain;
pub mod infrastructure;

// Generated protobuf types
pub mod proto {
    pub mod process_manager {
        tonic::include_proto!("process_manager");

        // Include file descriptor for reflection
        pub const FILE_DESCRIPTOR_SET: &[u8] =
            tonic::include_file_descriptor_set!("proto_descriptor");
    }
}
