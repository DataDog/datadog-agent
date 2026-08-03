// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Bazel builds use the generated Rust crate from
// `//pkg/proto/datadog/procmgr:procmgr_rust_proto` (same `process_manager.proto` as Go).
// This script exists only for `cargo build` / IDE workflows outside Bazel.

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo::rustc-check-cfg=cfg(bazel)");

    let out_dir = std::path::PathBuf::from(std::env::var("OUT_DIR").unwrap());
    tonic_prost_build::configure()
        .file_descriptor_set_path(out_dir.join("process_manager_descriptor.bin"))
        .compile_protos(
            &["../../proto/datadog/procmgr/process_manager.proto"],
            &["../../proto"],
        )?;
    Ok(())
}
