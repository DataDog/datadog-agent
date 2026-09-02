// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

fn main() {
    println!("cargo::rustc-check-cfg=cfg(bazel)");
    for cfg in [
        "procmgr_pr_all",
        "procmgr_pr_config_gate_secrets",
        "procmgr_pr_config_gate_core",
        "procmgr_pr_secret_backend_exec",
        "procmgr_pr_platform_unix_secret_backend",
        "procmgr_pr_platform_windows_resolve_executable",
        "procmgr_pr_platform_windows_secret_backend",
        "procmgr_pr_platform_windows_legacy_scm_env",
        "procmgr_pr_platform_windows_secret_backend_rights",
    ] {
        println!("cargo::rustc-check-cfg=cfg({cfg})");
    }
}
