// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Bazel builds use the generated Rust crate from
// `//pkg/proto/datadog/sds:sds_rust_proto` (rules_rust_prost, same
// `sds_result.proto` as Go). This script exists only for `cargo build` / IDE
// workflows outside Bazel and compiles the canonical proto directly (same
// pattern as pkg/procmgr/rust/build.rs).
//
// Canonical source: pkg/proto/datadog/sds/sds_result.proto
// Requires `protoc` on PATH (e.g. `brew install protobuf`).

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo::rustc-check-cfg=cfg(bazel)");

    let proto_root = "../../../../../proto";
    let proto_file = "../../../../../proto/datadog/sds/sds_result.proto";

    println!("cargo:rerun-if-changed={proto_file}");

    prost_build::compile_protos(&[proto_file], &[proto_root])?;

    Ok(())
}
