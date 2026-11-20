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

/// File the entrypoint writes into the fallback directory for this launch.
#[cfg(unix)]
const STAGING_TOKEN_FILE: &str = ".dd-procmgr-cleanup";

/// Remove `DD_PM_CLEANUP_DIR` after the children have stopped.
///
/// The entrypoint sets that variable to the one fallback directory this start
/// created, and `DD_PM_CLEANUP_TOKEN` to the token it wrote into that directory.
/// Removal also requires the directory to be owned by this process. A stale path
/// or a directory owned by someone else is left in place. Only the Linux
/// container entrypoint sets the variables, so other platforms do nothing.
#[cfg(unix)]
pub fn remove_startup_staging_dir() {
    let Ok(raw) = std::env::var("DD_PM_CLEANUP_DIR") else {
        return;
    };
    if raw.is_empty() {
        return;
    }
    let path = Path::new(&raw);
    let Ok(token) = std::env::var("DD_PM_CLEANUP_TOKEN") else {
        log::warn!(
            "refusing to remove DD_PM_CLEANUP_DIR {}: no cleanup token",
            path.display()
        );
        return;
    };
    if let Err(err) = remove_checked_staging_dir(path, &token) {
        log::warn!(
            "refusing to remove DD_PM_CLEANUP_DIR {}: {err}",
            path.display()
        );
    }
}

#[cfg(not(unix))]
pub fn remove_startup_staging_dir() {}

#[cfg(unix)]
fn remove_checked_staging_dir(path: &Path, token: &str) -> Result<(), String> {
    check_startup_staging_dir(path).map_err(str::to_string)?;
    check_staging_launch_token(path, token).map_err(str::to_string)?;
    make_tree_removable(path).map_err(|err| err.to_string())?;
    std::fs::remove_dir_all(path).map_err(|err| err.to_string())?;
    log::info!("removed staging directory {}", path.display());
    Ok(())
}

/// The token and the owner are what tie the path to the directory this launch created.
#[cfg(unix)]
fn check_staging_launch_token(path: &Path, token: &str) -> Result<(), &'static str> {
    use std::os::unix::fs::MetadataExt;
    if token.len() < 32
        || !token
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || byte == b'-')
    {
        return Err("cleanup token is not a launch token");
    }
    let meta = std::fs::symlink_metadata(path).map_err(|_| "path metadata could not be read")?;
    if meta.uid() != nix::unistd::Uid::effective().as_raw() {
        return Err("directory is not owned by this process");
    }
    let token_path = path.join(STAGING_TOKEN_FILE);
    let file_meta = match std::fs::symlink_metadata(&token_path) {
        Ok(meta) => meta,
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => {
            return Err("cleanup token file is missing");
        }
        Err(_) => return Err("cleanup token file could not be read"),
    };
    if file_meta.file_type().is_symlink() {
        return Err("cleanup token file is a symlink");
    }
    if !file_meta.file_type().is_file() {
        return Err("cleanup token file is not a regular file");
    }
    let bytes = std::fs::read(&token_path).map_err(|_| "cleanup token file could not be read")?;
    let stored = bytes.strip_suffix(b"\n").unwrap_or(&bytes);
    if stored != token.as_bytes() {
        return Err("cleanup token does not match this directory");
    }
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
    use super::{
        STAGING_TOKEN_FILE, STARTUP_STAGING_PREFIX, check_startup_staging_dir,
        remove_checked_staging_dir,
    };
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

        let token = "01234567-89ab-cdef-0123-456789abcdef";
        fs::write(ours.join(STAGING_TOKEN_FILE), format!("{token}\n")).unwrap();
        remove_checked_staging_dir(&ours, token).unwrap();
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

    #[test]
    fn refuses_a_directory_without_this_launch_token() {
        let root =
            std::env::temp_dir().join(format!("procmgr-staging-token-test-{}", std::process::id()));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).unwrap();
        let ours = root.join(staging_name());
        fs::create_dir_all(&ours).unwrap();
        let token = "0123456789abcdef0123456789abcdef";
        fs::write(ours.join(STAGING_TOKEN_FILE), format!("{token}\n")).unwrap();
        let wrong = "fedcba9876543210fedcba9876543210";

        assert!(remove_checked_staging_dir(&ours, wrong).is_err());
        assert!(ours.exists());
        assert!(remove_checked_staging_dir(&ours, "").is_err());
        assert!(ours.exists());
        fs::remove_file(ours.join(STAGING_TOKEN_FILE)).unwrap();
        assert!(remove_checked_staging_dir(&ours, token).is_err());
        assert!(ours.exists());

        let short = "short";
        fs::write(ours.join(STAGING_TOKEN_FILE), format!("{short}\n")).unwrap();
        assert!(remove_checked_staging_dir(&ours, short).is_err());
        assert!(ours.exists());

        fs::remove_file(ours.join(STAGING_TOKEN_FILE)).unwrap();
        let token_target = root.join("token-target");
        fs::write(&token_target, format!("{token}\n")).unwrap();
        std::os::unix::fs::symlink(&token_target, ours.join(STAGING_TOKEN_FILE)).unwrap();
        assert!(remove_checked_staging_dir(&ours, token).is_err());
        assert!(ours.exists());
        assert!(token_target.exists());

        fs::remove_file(ours.join(STAGING_TOKEN_FILE)).unwrap();
        fs::create_dir(ours.join(STAGING_TOKEN_FILE)).unwrap();
        assert_eq!(
            remove_checked_staging_dir(&ours, token).unwrap_err(),
            "cleanup token file is not a regular file"
        );
        assert!(ours.exists());
        let _ = fs::remove_dir_all(&root);
    }
}
