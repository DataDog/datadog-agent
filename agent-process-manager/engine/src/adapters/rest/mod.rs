//! REST API Driving Adapter
//!
//! Exposes use cases through RESTful HTTP API (JSON)

pub mod handlers;
pub mod router;
pub mod unix_socket;

pub use router::build_router;
pub use unix_socket::serve_on_unix_socket;
