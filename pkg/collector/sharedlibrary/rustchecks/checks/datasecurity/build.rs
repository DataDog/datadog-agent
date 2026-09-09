// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// cargo-only proto codegen into OUT_DIR; Bazel uses the sds_proto crate (see src/proto.rs).
// Requires `protoc` on PATH.

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo::rustc-check-cfg=cfg(bazel)");
    prost_build::compile_protos(
        &["../../../../../proto/datadog/sds/sds_result.proto"],
        &["../../../../../proto"],
    )?;
    Ok(())
}
