use crate::domain::DomainError;
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use tracing::{debug, warn};

/// Create runtime directories under /run/ with proper permissions
///
/// This matches systemd's RuntimeDirectory= behavior:
/// - Creates directories under /run/ (e.g., /run/datadog)
/// - Sets permissions to 0755
/// - Sets ownership if user/group specified (best effort)
/// - Cleans up on failure
///
/// # Arguments
///
/// * `directories` - List of directory names to create under /run/
/// * `user` - Optional user name for ownership
/// * `group` - Optional group name for ownership
///
/// # Returns
///
/// Vector of created paths for potential cleanup
pub fn create_runtime_directories(
    directories: &[String],
    user: Option<&str>,
    group: Option<&str>,
) -> Result<Vec<PathBuf>, DomainError> {
    let runtime_base = Path::new("/run");
    let mut created_paths = Vec::new();

    for dir_name in directories {
        let full_path = runtime_base.join(dir_name);

        // Create the directory with parents
        if let Err(e) = fs::create_dir_all(&full_path) {
            // Clean up any already created directories
            cleanup_directories(&created_paths);
            return Err(DomainError::InvalidCommand(format!(
                "Failed to create runtime directory {:?}: {}",
                full_path, e
            )));
        }

        // Set permissions to 0755
        if let Err(e) = fs::set_permissions(&full_path, fs::Permissions::from_mode(0o755)) {
            // Clean up on permission error
            cleanup_directories(&created_paths);
            let _ = fs::remove_dir_all(&full_path);
            return Err(DomainError::InvalidCommand(format!(
                "Failed to set permissions on {:?}: {}",
                full_path, e
            )));
        }

        // Set ownership if user/group specified (best effort - don't fail if it doesn't work)
        if user.is_some() || group.is_some() {
            if let Err(e) = set_directory_ownership(&full_path, user, group) {
                warn!(
                    path = ?full_path,
                    user = ?user,
                    group = ?group,
                    error = %e,
                    "Failed to set ownership on runtime directory (continuing anyway)"
                );
                // Don't fail the process start if ownership change fails
                // This allows non-root daemon to still work
            }
        }

        debug!(
            path = ?full_path,
            user = ?user,
            group = ?group,
            "Created runtime directory"
        );

        created_paths.push(full_path);
    }

    Ok(created_paths)
}

/// Clean up runtime directories (best effort)
fn cleanup_directories(paths: &[PathBuf]) {
    for path in paths {
        if let Err(e) = fs::remove_dir_all(path) {
            warn!(
                path = ?path,
                error = %e,
                "Failed to clean up runtime directory"
            );
        }
    }
}

/// Clean up runtime directories by name (public API for process stop)
///
/// This is called when a process stops to clean up its runtime directories.
/// Cleanup is best-effort and will not fail if directories don't exist.
///
/// # Arguments
///
/// * `directories` - List of directory names under /run/ to remove
pub fn cleanup_runtime_directories(directories: &[String]) {
    let runtime_base = Path::new("/run");

    for dir_name in directories {
        let full_path = runtime_base.join(dir_name);

        if let Err(e) = fs::remove_dir_all(&full_path) {
            // Only log if it's not "not found" (which is expected if already cleaned)
            if e.kind() != std::io::ErrorKind::NotFound {
                warn!(
                    path = ?full_path,
                    error = %e,
                    "Failed to clean up runtime directory"
                );
            }
        } else {
            debug!(path = ?full_path, "Cleaned up runtime directory");
        }
    }
}

/// Set directory ownership (requires appropriate privileges)
#[cfg(unix)]
fn set_directory_ownership(
    path: &Path,
    user: Option<&str>,
    group: Option<&str>,
) -> Result<(), String> {
    use std::ffi::CString;

    // Resolve user to UID
    let uid = if let Some(user_name) = user {
        resolve_user_to_uid(user_name)?
    } else {
        None
    };

    // Resolve group to GID
    let gid = if let Some(group_name) = group {
        resolve_group_to_gid(group_name)?
    } else {
        None
    };

    // Only call chown if we have something to change
    if uid.is_none() && gid.is_none() {
        return Ok(());
    }

    let path_cstr = CString::new(path.to_str().ok_or("Invalid path")?)
        .map_err(|e| format!("Failed to convert path to CString: {}", e))?;

    let uid_val = uid.unwrap_or(u32::MAX); // -1 means don't change
    let gid_val = gid.unwrap_or(u32::MAX); // -1 means don't change

    unsafe {
        if libc::chown(path_cstr.as_ptr(), uid_val, gid_val) != 0 {
            return Err(format!("chown failed: {}", std::io::Error::last_os_error()));
        }
    }

    Ok(())
}

/// Resolve username to UID
#[cfg(unix)]
fn resolve_user_to_uid(username: &str) -> Result<Option<u32>, String> {
    use std::ffi::CString;

    // Try parsing as numeric UID first
    if let Ok(uid) = username.parse::<u32>() {
        return Ok(Some(uid));
    }

    // Look up by username
    let username_cstr = CString::new(username).map_err(|e| format!("Invalid username: {}", e))?;

    unsafe {
        let pwd = libc::getpwnam(username_cstr.as_ptr());
        if pwd.is_null() {
            return Err(format!("User not found: {}", username));
        }
        Ok(Some((*pwd).pw_uid))
    }
}

/// Resolve group name to GID
#[cfg(unix)]
fn resolve_group_to_gid(groupname: &str) -> Result<Option<u32>, String> {
    use std::ffi::CString;

    // Try parsing as numeric GID first
    if let Ok(gid) = groupname.parse::<u32>() {
        return Ok(Some(gid));
    }

    // Look up by group name
    let groupname_cstr =
        CString::new(groupname).map_err(|e| format!("Invalid group name: {}", e))?;

    unsafe {
        let grp = libc::getgrnam(groupname_cstr.as_ptr());
        if grp.is_null() {
            return Err(format!("Group not found: {}", groupname));
        }
        Ok(Some((*grp).gr_gid))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_runtime_directories_validation() {
        // Test that function exists and has correct signature
        let empty_dirs: Vec<String> = vec![];
        let result = create_runtime_directories(&empty_dirs, None, None);
        assert!(result.is_ok());
        assert_eq!(result.unwrap().len(), 0);
    }
}
