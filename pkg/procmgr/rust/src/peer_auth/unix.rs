// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

/// RunPrivilegedCommand peer authorization is not implemented on Unix yet (PR 4).
pub fn authorize_par_caller(_client_pid: u32) -> bool {
    false
}
