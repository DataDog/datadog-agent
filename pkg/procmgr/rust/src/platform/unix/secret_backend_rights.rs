// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Executable permission checks for `secret_backend_command` on Unix.
//!
//! Mirrors `pkg/util/filesystem/rights_nix.go` (`CheckRights`).

use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use anyhow::{Context, Result, bail};
use nix::unistd::{AccessFlags, access};

/// Validate that only the owner can execute the backend, optionally allowing group execute.
pub(crate) fn check_secret_backend_command_rights(
    path: &str,
    allow_group_exec: bool,
) -> Result<()> {
    let path_obj = Path::new(path);
    if !path_obj.is_file() {
        bail!("invalid executable '{path}', can't stat it: no such file");
    }

    let metadata = std::fs::metadata(path_obj)
        .with_context(|| format!("invalid executable '{path}', can't stat it"))?;
    let mode = metadata.permissions().mode();

    if allow_group_exec {
        if mode & 0o027 != 0 {
            bail!(
                "invalid executable '{path}', 'others' have rights on it or 'group' has write permissions on it"
            );
        }
    } else if mode & 0o077 != 0 {
        bail!("invalid executable '{path}', 'group' or 'others' have rights on it");
    }

    access(path_obj, AccessFlags::X_OK)
        .with_context(|| format!("invalid executable '{path}', can't access it"))?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::PermissionsExt;

    #[test]
    fn rejects_group_or_other_bits_when_group_exec_disabled() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("backend.sh");
        std::fs::write(&path, "#!/bin/sh\n").unwrap();
        let mut perms = std::fs::metadata(&path).unwrap().permissions();
        perms.set_mode(0o755);
        std::fs::set_permissions(&path, perms).unwrap();

        let err = check_secret_backend_command_rights(path.to_str().unwrap(), false).unwrap_err();
        assert!(
            err.to_string().contains("'group' or 'others' have rights"),
            "unexpected error: {err:#}"
        );
    }

    #[test]
    fn allows_group_execute_when_enabled() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("backend.sh");
        std::fs::write(&path, "#!/bin/sh\n").unwrap();
        let mut perms = std::fs::metadata(&path).unwrap().permissions();
        perms.set_mode(0o750);
        std::fs::set_permissions(&path, perms).unwrap();

        check_secret_backend_command_rights(path.to_str().unwrap(), true).unwrap();
    }
}
