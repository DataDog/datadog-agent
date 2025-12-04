//! REST API Driving Adapter
//!
//! Exposes use cases through RESTful HTTP API (JSON)
//!
//! Supports multiple transports:
//! - Unix sockets (Linux/macOS)
//! - TCP (all platforms, required for Windows)

pub mod handlers;
pub mod router;
pub mod unix_socket;

pub use router::build_router;
pub use unix_socket::{serve_on_tcp, serve_on_unix_socket};
