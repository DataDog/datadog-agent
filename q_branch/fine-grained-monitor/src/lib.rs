//! Fine-grained container resource monitoring for datadog-agent development.
//!
//! This crate provides a tool to capture detailed resource metrics (memory, CPU)
//! from all containers on a Kubernetes node and write them to a Parquet file for
//! post-hoc analysis.
//!
//! ## Architecture
//!
//! The monitor consists of three main components:
//!
//! 1. **Container Discovery** (`discovery` module) - Scans the cgroup filesystem
//!    to discover running containers without external dependencies.
//!
//! 2. **Observer** (`observer` module) - Collects detailed metrics from procfs
//!    and cgroup interfaces for each discovered container.
//!
//! 3. **Capture Pipeline** (via `lading_capture`) - Accumulates metrics and
//!    writes them to Parquet with columnar compression.
//!
//! ## Usage
//!
//! Run as a binary on a Kubernetes node (typically via DaemonSet):
//!
//! ```bash
//! fine-grained-monitor \
//!   --output /data/metrics.parquet \
//!   --interval-ms 1000 \
//!   --compression-level 3
//! ```

pub mod discovery;
pub mod index;
pub mod kubernetes;
pub mod metrics_viewer;
pub mod observer;
pub mod sidecar;
