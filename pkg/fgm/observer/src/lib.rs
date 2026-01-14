// Copyright 2025 Datadog, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//! Fine-Grained Metrics Observer Library
//!
//! This library provides a C FFI interface for collecting fine-grained
//! container metrics from Linux cgroups and procfs. It wraps the lading
//! observer APIs to sample memory, CPU, and PSI metrics at the container level.
//!
//! # FFI Interface
//!
//! - `fgm_init()` - Initialize the library (creates Tokio runtime)
//! - `fgm_shutdown()` - Shutdown the library
//! - `fgm_sample_container()` - Sample metrics for a single container
//!
//! # Usage
//!
//! ```c
//! // Initialize
//! if (fgm_init() != 0) {
//!     // handle error
//! }
//!
//! // Sample container
//! fgm_sample_container("/sys/fs/cgroup/system.slice/docker-abc123.scope",
//!                      12345, callback, context);
//!
//! // Cleanup
//! fgm_shutdown();
//! ```

pub mod ffi;
pub mod metrics_bridge;
pub mod observer;
