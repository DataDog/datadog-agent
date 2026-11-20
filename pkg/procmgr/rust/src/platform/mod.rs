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
#[cfg(unix)]
use std::path::{Component, Path};

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

/// Directory name prefix of a fallback `mktemp` created by `procmgr-all-in-one`.
#[cfg(unix)]
const STARTUP_STAGING_PREFIX: &str = "procmgr-processes.d.";

/// Remove `DD_PM_CLEANUP_DIR` after the children have stopped.
///
/// The entrypoint sets that variable to the one fallback directory this start
/// created. The name check refuses a primary path, a caller-supplied path, and
/// anything that is not that `mktemp` directory. Only the Linux container
/// entrypoint sets the variable, so other platforms do nothing.
#[cfg(unix)]
pub fn remove_startup_staging_dir() {
    let Ok(raw) = std::env::var("DD_PM_CLEANUP_DIR") else {
        return;
    };
    if raw.is_empty() {
        return;
    }
    let path = Path::new(&raw);
    if let Err(err) = remove_checked_staging_dir(path) {
        log::warn!(
            "refusing to remove DD_PM_CLEANUP_DIR {}: {err}",
            path.display()
        );
    }
}

#[cfg(not(unix))]
pub fn remove_startup_staging_dir() {}

#[cfg(unix)]
fn remove_checked_staging_dir(path: &Path) -> Result<(), String> {
    check_startup_staging_dir(path).map_err(str::to_string)?;
    make_tree_removable(path).map_err(|err| err.to_string())?;
    std::fs::remove_dir_all(path).map_err(|err| err.to_string())?;
    log::info!("removed staging directory {}", path.display());
    Ok(())
}

#[cfg(unix)]
fn check_startup_staging_dir(path: &Path) -> Result<(), &'static str> {
    if !path.is_absolute() {
        return Err("path is not absolute");
    }
    // lstat follows a final symlink when the path ends in "/" or "/.", and file_name()
    // ignores both suffixes, so refuse them before the name and symlink checks.
    let raw = path.as_os_str().as_encoded_bytes();
    if raw.ends_with(b"/") || raw.ends_with(b"/.") {
        return Err("path ends with a separator or .");
    }
    if path
        .components()
        .any(|component| component == Component::ParentDir)
    {
        return Err("path contains ..");
    }
    let Some(name) = path.file_name().and_then(|name| name.to_str()) else {
        return Err("path has no directory name");
    };
    let Some(suffix) = name.strip_prefix(STARTUP_STAGING_PREFIX) else {
        return Err("directory was not created as a fallback staging directory");
    };
    if suffix.len() < 6 || !suffix.chars().all(|ch| ch.is_ascii_alphanumeric()) {
        return Err("directory name is not a mktemp suffix");
    }
    match std::fs::symlink_metadata(path) {
        Ok(meta) if meta.file_type().is_symlink() => Err("path is a symlink"),
        Ok(meta) if meta.is_dir() => Ok(()),
        Ok(_) => Err("path is not a directory"),
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => Err("path does not exist"),
        Err(_) => Err("path metadata could not be read"),
    }
}

/// `chmod u+w` for this process's tree. A mode 0555 copy is otherwise not removable.
#[cfg(unix)]
fn make_tree_removable(path: &Path) -> std::io::Result<()> {
    use std::os::unix::fs::PermissionsExt;
    let meta = std::fs::symlink_metadata(path)?;
    if meta.file_type().is_symlink() {
        return Ok(());
    }
    let mut perms = meta.permissions();
    perms.set_mode(perms.mode() | 0o200);
    std::fs::set_permissions(path, perms)?;
    if !meta.is_dir() {
        return Ok(());
    }
    for entry in std::fs::read_dir(path)? {
        make_tree_removable(&entry?.path())?;
    }
    Ok(())
}

#[cfg(all(test, unix))]
mod startup_staging_tests {
    use super::{STARTUP_STAGING_PREFIX, check_startup_staging_dir, remove_checked_staging_dir};
    use std::fs;
    use std::path::Path;

    fn staging_name() -> String {
        format!("{STARTUP_STAGING_PREFIX}{}", &uuid_like()[..6])
    }

    fn uuid_like() -> String {
        // Eight hex digits for any pid, so the six-character slice cannot panic.
        format!("{:08x}", std::process::id())
    }

    #[test]
    fn removes_only_the_named_fallback_directory() {
        let root =
            std::env::temp_dir().join(format!("procmgr-staging-test-{}", std::process::id()));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).unwrap();
        let ours = root.join(staging_name());
        let nested = ours.join("services").join("custom");
        fs::create_dir_all(&nested).unwrap();
        fs::write(nested.join("run"), b"#!/bin/sh\n").unwrap();
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&nested, fs::Permissions::from_mode(0o555)).unwrap();
        }
        let other = root.join("procmgr-processes.d");
        fs::create_dir_all(&other).unwrap();

        remove_checked_staging_dir(&ours).unwrap();
        assert!(!ours.exists());
        assert!(other.exists());
        assert!(check_startup_staging_dir(&other).is_err());
        assert!(check_startup_staging_dir(Path::new("procmgr-processes.d.abcdef")).is_err());
        assert!(
            check_startup_staging_dir(&root.join("procmgr-processes.d.abcdef").join("..")).is_err()
        );
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn refuses_symlink_named_as_fallback_even_with_trailing_separator() {
        let root =
            std::env::temp_dir().join(format!("procmgr-staging-link-test-{}", std::process::id()));
        let _ = fs::remove_dir_all(&root);
        let target = root.join("target");
        fs::create_dir_all(&target).unwrap();
        fs::write(target.join("keep"), b"x").unwrap();
        let link = root.join(staging_name());
        std::os::unix::fs::symlink(&target, &link).unwrap();

        assert!(check_startup_staging_dir(&link).is_err());
        for raw in [
            format!("{}/", link.display()),
            format!("{}/.", link.display()),
        ] {
            assert!(check_startup_staging_dir(Path::new(&raw)).is_err(), "{raw}");
        }
        assert!(target.join("keep").exists());
        let _ = fs::remove_dir_all(&root);
    }
}
