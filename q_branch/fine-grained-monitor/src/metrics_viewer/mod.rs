//! Metrics viewer library module.
//!
//! Provides parquet loading, HTTP server, and study framework for
//! interactive timeseries visualization.
//!
//! # Architecture
//!
//! - `data` - Parquet loading and metric discovery (REQ-MV-002, REQ-MV-003)
//! - `server` - HTTP server and API handlers (REQ-MV-001)
//! - `studies` - Analysis framework with Study trait (REQ-MV-007)
//!
//! # Usage
//!
//! ```rust,ignore
//! use fine_grained_monitor::metrics_viewer::{data, server};
//!
//! let data = data::load_parquet_files(&["metrics.parquet"])?;
//! let config = server::ServerConfig::default();
//! server::run_server(data, config).await?;
//! ```

pub mod data;
pub mod lazy_data;
pub mod server;
pub mod studies;

pub use lazy_data::LazyDataStore;
pub use server::{run_server, ServerConfig};
