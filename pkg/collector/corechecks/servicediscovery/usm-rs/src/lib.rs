//! Universal Service Metadata (USM) Detection Library
//!
//! This library provides functionality to detect service names and metadata from running
//! processes by analyzing command-line arguments, environment variables, and filesystem contents.
//!
//! It supports multiple programming languages and frameworks:
//! - Java (Spring Boot, JEE servers like Tomcat, JBoss, WebLogic, WebSphere)
//! - Python (Django, Flask, Gunicorn, uWSGI)
//! - Node.js (package.json based detection)
//! - PHP (Laravel, Composer)
//! - Ruby (Rails)
//! - .NET (assembly detection)

pub mod context;
pub mod detectors;
pub mod error;
pub mod filesystem;
pub mod frameworks;
pub mod language;
pub mod metadata;
pub mod platform;
pub mod service;
pub mod utils;

// Re-export main types for easier usage
pub use context::DetectionContext;
pub use error::{UsmError, UsmResult};
pub use language::Language;
pub use metadata::{ServiceMetadata, ServiceNameSource};
pub use service::extract_service_metadata;

// FFI module for C integration
pub mod ffi;