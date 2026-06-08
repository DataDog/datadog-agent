// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Generates the Rust client for the minimal AgentSecure remote-config subset.
// The message definitions are imported from the canonical proto under
// pkg/proto, so they cannot drift from the agent.
//
// Note: this is the cargo build path. Under bazel the protos are compiled by
// build rules and the BUILD.bazel must declare the remoteconfig proto_library
// as a dependency.
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let manifest = std::path::PathBuf::from(std::env::var("CARGO_MANIFEST_DIR")?);
    let pkg_proto = manifest.join("../../../../pkg/proto");

    tonic_prost_build::configure()
        .build_server(false)
        .compile_protos(
            &[manifest.join("proto/rc.proto")],
            &[manifest.join("proto"), pkg_proto],
        )?;
    Ok(())
}
