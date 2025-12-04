//! gRPC Driving Adapter
//!
//! Exposes use cases through gRPC protocol (Protobuf)
//!
//! Supports multiple transports:
//! - Unix sockets (Linux/macOS)
//! - TCP (all platforms, required for Windows)

pub mod mappers;
pub mod proto_conversions;
pub mod service;
pub mod unix_socket;

pub use service::ProcessManagerService;
pub use unix_socket::{
    serve_on_tcp, serve_on_tcp_with_health, serve_on_unix_socket, serve_on_unix_socket_with_health,
};
