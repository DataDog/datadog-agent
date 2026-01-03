//! Metrics viewer library module.
//!
//! Provides lazy parquet loading, HTTP server, and study framework for
//! interactive timeseries visualization.
//!
//! # Architecture
//!
//! - `data` - Core data types (TimeseriesPoint, ContainerInfo, etc.)
//! - `lazy_data` - Lazy parquet loading with on-demand data fetching (REQ-MV-012)
//! - `server` - HTTP server and API handlers (REQ-MV-001)
//! - `studies` - Analysis framework with Study trait (REQ-MV-006)

pub mod data;
pub mod lazy_data;
pub mod server;
pub mod studies;

pub use lazy_data::LazyDataStore;
pub use server::{run_server, ServerConfig};
