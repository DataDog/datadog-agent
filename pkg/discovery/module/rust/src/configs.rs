// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Discovery of on-disk config files for a target process.
//!
//! Counterpart to `procfs::fd::get_log_files`: log files are typically held
//! open and discoverable via `/proc/<pid>/fd/`, but config files are read
//! once at startup and closed. We instead inspect the process command line
//! for well-known "config" flags (`-c`, `--config`, `--config-file`, etc.)
//! and treat the adjacent argument (or the `--flag=value` inline form) as
//! a candidate path. The candidate is then verified to actually exist inside
//! the target process's mount namespace via the caller-provided `SubDirFs`
//! (typically rooted at `/proc/<pid>/root/`), which uses cap-std to reject
//! `..` traversal and symlink escapes.
//!
//! When the cmdline-scan finds nothing, we fall back to a small table of
//! well-known paths keyed off `argv[0]`'s basename. This is necessary for
//! services that take their config as a positional argument and/or overwrite
//! argv at startup, so the original path is unrecoverable from
//! `/proc/<pid>/cmdline` — `redis-server` is the canonical case (it rewrites
//! argv to display the listen address). Follow-ups can extend the table
//! and add env-var-based location hints (Spring `SPRING_CONFIG_LOCATIONS`,
//! `JAVA_TOOL_OPTIONS -Dconfig.file=…`).

use std::collections::HashSet;

use crate::fs::SubDirFs;
use crate::procfs::Cmdline;

/// Short-form flags (`-c value`, `-f value`) that introduce a config path.
const SHORT_CONFIG_FLAGS: &[&str] = &["-c", "-f"];

/// Long-form flag names (without the leading `--`) recognised in both
/// separated (`--config value`) and inline (`--config=value`) forms.
const LONG_CONFIG_FLAGS: &[&str] = &["config", "config-file", "conf-file", "configFile"];

/// Per-exe fallback table: matched against `argv[0]`'s basename when the
/// cmdline-scan finds nothing. Paths are tried in order and all that exist
/// inside the sandbox are reported.
const WELL_KNOWN_CONFIGS: &[(&str, &[&str])] =
    &[("redis-server", &["/etc/redis/redis.conf", "/etc/redis.conf"])];

/// Extract config-file paths referenced by the process command line.
///
/// Returns absolute paths that (a) appear after a known config flag in
/// `cmdline` and (b) actually exist inside the `fs` sandbox. The result
/// is deduplicated and order-preserving (first occurrence wins).
///
/// Relative paths are skipped: the process's cwd is ambiguous in this
/// context (cap-std would resolve relative paths against the sandbox root,
/// not the process's cwd, which would silently produce wrong results).
///
/// When the cmdline-scan finds nothing, falls back to `WELL_KNOWN_CONFIGS`
/// keyed off `argv[0]`'s basename, so daemons that take a positional config
/// path or rewrite argv (e.g. `redis-server`) can still be discovered.
pub fn get_config_files(cmdline: &Cmdline, fs: &SubDirFs) -> Vec<String> {
    if cmdline.is_empty() {
        return Vec::new();
    }

    let mut seen = HashSet::new();
    let mut result = Vec::new();
    for candidate in extract_candidates(cmdline) {
        if !candidate.starts_with('/') {
            continue;
        }
        // Reject `..` components. The kernel clamps `..` at the filesystem
        // root, so cap-std's sandbox is not actually escapable through this
        // path — but the reported string is later handed to a downstream
        // ingester that may re-resolve it outside the sandbox, so we keep
        // the reported path free of traversal segments.
        if candidate.split('/').any(|c| c == "..") {
            continue;
        }
        if !seen.insert(candidate.clone()) {
            continue;
        }
        if fs.exists(&candidate) {
            result.push(candidate);
        }
    }

    if result.is_empty() {
        result.extend(well_known_fallback(cmdline, fs));
    }

    result
}

/// Probe the well-known-paths table for the current `argv[0]`. Returns the
/// subset of registered paths that exist inside the sandbox.
fn well_known_fallback(cmdline: &Cmdline, fs: &SubDirFs) -> Vec<String> {
    let Some(exe) = argv0_basename(cmdline) else {
        return Vec::new();
    };
    WELL_KNOWN_CONFIGS
        .iter()
        .find(|(name, _)| *name == exe)
        .map(|(_, paths)| {
            paths
                .iter()
                .filter(|p| fs.exists(p))
                .map(|p| (*p).to_string())
                .collect()
        })
        .unwrap_or_default()
}

/// Basename of `argv[0]`, or `None` if cmdline is empty / argv[0] is empty.
fn argv0_basename(cmdline: &Cmdline) -> Option<&str> {
    let argv0 = cmdline.args().next()?;
    if argv0.is_empty() {
        return None;
    }
    Some(argv0.rsplit('/').next().unwrap_or(argv0))
}

/// Parse cmdline args, returning all path-like tokens that follow a known
/// config flag (separated form) or are attached via `--flag=value`.
fn extract_candidates(cmdline: &Cmdline) -> Vec<String> {
    let mut out = Vec::new();
    let mut iter = cmdline.args().peekable();
    while let Some(arg) = iter.next() {
        // Inline form: `--config=/etc/myapp.yaml`
        if let Some(rest) = arg.strip_prefix("--")
            && let Some((key, value)) = rest.split_once('=')
            && LONG_CONFIG_FLAGS.contains(&key)
            && !value.is_empty()
        {
            out.push(value.to_string());
            continue;
        }

        // Separated form: `-c /etc/redis/redis.conf` or `--config /path`
        if is_separated_config_flag(arg)
            && let Some(&value) = iter.peek()
            && !value.is_empty()
            && !value.starts_with('-')
        {
            out.push(value.to_string());
            iter.next();
        }
    }
    out
}

fn is_separated_config_flag(arg: &str) -> bool {
    SHORT_CONFIG_FLAGS.contains(&arg)
        || arg
            .strip_prefix("--")
            .is_some_and(|k| LONG_CONFIG_FLAGS.contains(&k))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::cmdline;

    fn host_root_fs() -> SubDirFs {
        // Any existing directory works for the existence-check tests that
        // either reach absolute host paths (Linux) or never hit the FS.
        SubDirFs::new("/").expect("open / as SubDirFs root")
    }

    #[test]
    fn separated_short_flag() {
        let cmd = cmdline!["redis-server", "-c", "/etc/redis/redis.conf"];
        assert_eq!(extract_candidates(&cmd), vec!["/etc/redis/redis.conf"]);
    }

    #[test]
    fn separated_long_flag() {
        let cmd = cmdline!["mongod", "--config", "/etc/mongod.conf"];
        assert_eq!(extract_candidates(&cmd), vec!["/etc/mongod.conf"]);
    }

    #[test]
    fn inline_long_flag() {
        let cmd = cmdline!["myapp", "--config=/etc/myapp.yaml"];
        assert_eq!(extract_candidates(&cmd), vec!["/etc/myapp.yaml"]);
    }

    #[test]
    fn camel_case_long_flag() {
        let cmd = cmdline!["mongod", "--configFile=/etc/mongod.conf"];
        assert_eq!(extract_candidates(&cmd), vec!["/etc/mongod.conf"]);
    }

    #[test]
    fn multiple_flags_preserve_order() {
        let cmd = cmdline![
            "kafka-server",
            "-f",
            "/etc/kafka/server.properties",
            "--config-file",
            "/etc/kafka/extra.conf",
        ];
        assert_eq!(
            extract_candidates(&cmd),
            vec!["/etc/kafka/server.properties", "/etc/kafka/extra.conf",],
        );
    }

    #[test]
    fn flag_with_no_following_value_is_ignored() {
        let cmd = cmdline!["foo", "--config"];
        assert!(extract_candidates(&cmd).is_empty());
    }

    #[test]
    fn flag_followed_by_another_flag_is_ignored() {
        // `-c -f /etc/x.conf` should NOT treat `-f` as `-c`'s value;
        // `-f /etc/x.conf` is then recognised on its own.
        let cmd = cmdline!["foo", "-c", "-f", "/etc/x.conf"];
        assert_eq!(extract_candidates(&cmd), vec!["/etc/x.conf"]);
    }

    #[test]
    fn unknown_flag_is_ignored() {
        let cmd = cmdline!["foo", "--bogus", "/etc/x.conf"];
        assert!(extract_candidates(&cmd).is_empty());
    }

    #[test]
    fn inline_with_empty_value_is_ignored() {
        let cmd = cmdline!["foo", "--config="];
        assert!(extract_candidates(&cmd).is_empty());
    }

    #[test]
    fn empty_cmdline_returns_empty() {
        let cmd = cmdline![];
        assert!(get_config_files(&cmd, &host_root_fs()).is_empty());
    }

    #[test]
    fn relative_paths_are_skipped() {
        let cmd = cmdline!["app", "-c", "relative/path.yaml"];
        assert!(get_config_files(&cmd, &host_root_fs()).is_empty());
    }

    #[test]
    fn dot_dot_components_are_rejected() {
        // cap-std rejects `..` traversal from the sandbox `Dir`, so the
        // candidate never resolves to an existing file even if the agent
        // has access to the target on the host.
        let cmd = cmdline!["app", "-c", "/etc/../../etc/passwd"];
        assert!(get_config_files(&cmd, &host_root_fs()).is_empty());

        let cmd = cmdline!["app", "--config=/..//root/.bashrc"];
        assert!(get_config_files(&cmd, &host_root_fs()).is_empty());
    }

    // Only runs on Linux because the temp-file path under `/tmp` is
    // resolved as a relative path inside the SubDirFs sandbox; the test
    // assumes the sandbox is rooted at `/` (the host's root view).
    #[cfg(target_os = "linux")]
    #[test]
    fn existence_filter_drops_missing_paths_and_dedupes() {
        use std::fs;
        use tempfile::TempDir;

        let temp_dir = TempDir::new().expect("create temp dir");
        let real = temp_dir.path().join("real.conf");
        fs::write(&real, "key=value\n").expect("write conf");
        let real_str = real.to_str().expect("utf-8 path").to_string();

        let cmd = Cmdline::from(
            &[
                "app",
                "-c",
                real_str.as_str(),
                "--config",
                real_str.as_str(),
                "-f",
                "/this/path/does/not/exist.conf",
            ][..],
        );

        let got = get_config_files(&cmd, &host_root_fs());
        assert_eq!(got.len(), 1, "expected dedup + existence filter, got: {got:?}");
        assert!(got.first().is_some_and(|p| p.ends_with("real.conf")));
    }

    #[test]
    fn argv0_basename_handles_paths_and_plain_names() {
        assert_eq!(
            argv0_basename(&cmdline!["redis-server"]),
            Some("redis-server"),
        );
        assert_eq!(
            argv0_basename(&cmdline!["/usr/bin/redis-server", "127.0.0.1:6379"]),
            Some("redis-server"),
        );
        assert_eq!(argv0_basename(&cmdline![]), None);
    }

    #[test]
    fn well_known_fallback_only_fires_for_known_exes() {
        // Non-redis exe → no fallback even if cmdline-scan is empty.
        let cmd = cmdline!["nginx", "-g", "daemon off;"];
        assert!(get_config_files(&cmd, &host_root_fs()).is_empty());
    }

    // Drives the redis fallback by rooting the sandbox at a tempdir that
    // mimics `/etc/redis/redis.conf`, mirroring the technique used by
    // `existence_filter_drops_missing_paths_and_dedupes`.
    #[cfg(target_os = "linux")]
    #[test]
    fn well_known_fallback_finds_redis_conf() {
        use std::fs;
        use tempfile::TempDir;

        let temp_dir = TempDir::new().expect("create temp dir");
        let redis_dir = temp_dir.path().join("etc").join("redis");
        fs::create_dir_all(&redis_dir).expect("create /etc/redis");
        fs::write(redis_dir.join("redis.conf"), "port 6379\n").expect("write redis.conf");

        let sandboxed = SubDirFs::new(temp_dir.path()).expect("open sandbox");

        // redis-server rewrites argv to the listen address; the original
        // positional `/etc/redis/redis.conf` is gone — but argv[0] still
        // says `redis-server`, which is enough for the fallback to fire.
        let cmd = cmdline!["redis-server", "127.0.0.1:6379"];
        assert_eq!(
            get_config_files(&cmd, &sandboxed),
            vec!["/etc/redis/redis.conf".to_string()],
        );
    }

    // If the cmdline-scan already found a path, the well-known table is
    // not consulted — avoids dupes and keeps behaviour predictable when
    // the operator passes an explicit `-c /custom/redis.conf`.
    #[cfg(target_os = "linux")]
    #[test]
    fn well_known_fallback_skipped_when_cmdline_already_matched() {
        use std::fs;
        use tempfile::TempDir;

        let temp_dir = TempDir::new().expect("create temp dir");
        // Create both the explicit cmdline path and the well-known path.
        let explicit = temp_dir.path().join("custom.conf");
        fs::write(&explicit, "x\n").expect("write custom.conf");
        let redis_dir = temp_dir.path().join("etc").join("redis");
        fs::create_dir_all(&redis_dir).expect("create /etc/redis");
        fs::write(redis_dir.join("redis.conf"), "y\n").expect("write redis.conf");

        let sandboxed = SubDirFs::new(temp_dir.path()).expect("open sandbox");

        let explicit_abs = format!("/{}", explicit.strip_prefix(temp_dir.path()).unwrap().display());
        let cmd = Cmdline::from(&["redis-server", "-c", explicit_abs.as_str()][..]);
        assert_eq!(get_config_files(&cmd, &sandboxed), vec![explicit_abs]);
    }
}
