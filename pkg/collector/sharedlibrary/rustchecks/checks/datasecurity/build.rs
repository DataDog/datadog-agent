// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Compiles the vendored copy of sds_result_payload.proto for `cargo build` / IDE
// workflows (same pattern as pkg/procmgr/rust/build.rs). Canonical source:
// pkg/proto/datadog/sds/sds_result_payload.proto
//
// Requires `protoc` on PATH (e.g. `brew install protobuf`).

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_root = "proto";
    let proto_file = "proto/datadog/sds/sds_result_payload.proto";

    println!("cargo:rerun-if-changed={proto_file}");

    prost_build::compile_protos(&[proto_file], &[proto_root])?;

    Ok(())
}
