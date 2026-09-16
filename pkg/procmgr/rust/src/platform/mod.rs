// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(unix)]
mod unix;
#[cfg(windows)]
mod windows;

#[cfg(unix)]
pub use self::unix::*;

#[cfg(windows)]
pub use self::windows::*;

use crate::spawn::SpawnProfile;

/// Initial status user for a managed process. On Windows, also returns a cached agent
/// spawn credential when the profile needs one.
#[cfg(unix)]
pub(crate) fn initial_spawn_identity(process_name: &str, profile: SpawnProfile) -> String {
    unix::intended_spawn_user(process_name, profile)
}

#[cfg(windows)]
pub(crate) fn initial_spawn_identity(
    process_name: &str,
    profile: SpawnProfile,
) -> (String, Option<windows::SpawnCredential>) {
    windows::resolve_initial_spawn_identity(process_name, profile)
}
