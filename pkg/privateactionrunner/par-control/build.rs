// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Bazel builds use `//pkg/proto/datadog/procmgr:procmgr_rust_proto` instead; this
// script only serves `cargo build` and IDE workflows.

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo::rustc-check-cfg=cfg(bazel)");
    tonic_prost_build::configure().compile_protos(
        &["../../proto/datadog/procmgr/process_manager.proto"],
        &["../../proto"],
    )?;
    Ok(())
}
