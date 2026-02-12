// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Correctness
#![deny(clippy::indexing_slicing)]
#![deny(clippy::string_slice)]
#![deny(clippy::cast_possible_wrap)]
#![deny(clippy::undocumented_unsafe_blocks)]
// Panicking code
#![deny(clippy::unwrap_used)]
#![deny(clippy::expect_used)]
#![deny(clippy::panic)]
#![deny(clippy::unimplemented)]
#![deny(clippy::todo)]
// Debug code that shouldn't be in production
#![deny(clippy::dbg_macro)]
#![deny(clippy::print_stdout)]
#![deny(clippy::print_stderr)]

mod apm;
mod binary;
pub mod cli;
pub mod config;
mod envs;
mod ephemeral;
mod errors;
pub mod ffi;
mod fs;
mod injector;
mod language;
mod netns;
mod params;
mod ports;
mod procfs;
mod service_name;
mod services;
mod tracer_metadata;
mod ust;

#[cfg(test)]
pub(crate) mod test_utils;

// Re-export the public API
pub use language::Language;
pub use params::Params;
pub use services::{Service, ServicesResponse, get_services};
pub use tracer_metadata::TracerMetadata;
pub use ust::UST;
