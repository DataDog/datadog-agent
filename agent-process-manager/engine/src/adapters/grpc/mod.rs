//! gRPC Driving Adapter
//!
//! Exposes use cases through gRPC protocol (Protobuf)

pub mod mappers;
pub mod proto_conversions;
pub mod service;
pub mod unix_socket;

pub use service::ProcessManagerService;
pub use unix_socket::{serve_on_unix_socket, serve_on_unix_socket_with_health};
