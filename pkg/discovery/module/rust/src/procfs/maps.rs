// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::fs::File;
use std::io::{BufRead, BufReader, Read};
use std::sync::LazyLock;

use memchr::memmem::Finder;

use super::root_path;

const MAPS_READ_LIMIT: u64 = 4 * 1024 * 1024 * 1024; // 4GiB

// Pre-built finders for NVIDIA-specific GPU libraries using memchr for fast substring search
static GPU_LIB_FINDERS: LazyLock<[Finder<'static>; 7]> = LazyLock::new(|| {
    [
        Finder::new(b"libcuda.so"),
        Finder::new(b"libcudart.so"),
        Finder::new(b"libnvidia-ml.so"),
        Finder::new(b"libnvrtc.so"),
        Finder::new(b"libcudnn.so"),
        Finder::new(b"libcublas.so"),
        Finder::new(b"libnccl.so"),
    ]
});

static DDTRACE_FINDER: LazyLock<Finder<'static>> = LazyLock::new(|| Finder::new(b"/ddtrace/"));

/// Consolidated information extracted from a single scan of /proc/[pid]/maps.
#[derive(Debug, Default)]
pub struct MapsInfo {
    /// Whether the APM injector launcher.preload.so is loaded.
    pub has_apm_injector: bool,
    /// Whether any NVIDIA GPU compute libraries (libcuda, libcudart, etc.) are loaded.
    pub has_gpu_nvidia: bool,
    /// Whether the Python ddtrace library is loaded.  Since the ddtrace package
    /// uses native libraries, the paths of these libraries will show up in
    /// /proc/$PID/maps.
    ///
    /// We look for the "/ddtrace/" part of the path.  We do not look for the
    /// "/site-packages/" part since some environments (such as pyinstaller) may
    /// not use that exact name.
    ///
    /// For example:
    /// 7aef453fc000-7aef453ff000 rw-p 0004c000 fc:06 7895473  /home/foo/.local/lib/python3.10/site-packages/ddtrace/internal/_encoding.cpython-310-x86_64-linux-gnu.so
    /// 7aef45400000-7aef45459000 r--p 00000000 fc:06 7895588  /home/foo/.local/lib/python3.10/site-packages/ddtrace/internal/datadog/profiling/libdd_wrapper.so
    pub has_ddtrace: bool,
    /// Whether the Datadog.Trace.dll (.NET) is loaded.  This detects cases
    /// where an older version of Datadog.Trace is used for manual
    /// instrumentation without enabling auto-instrumentation.  Note that this
    /// does not work for single-file deployments.
    ///
    /// For example:
    /// 785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267  /home/foo/.../publish/Datadog.Trace.dll
    pub has_datadog_trace_dll: bool,
    /// Whether the .NET System.Runtime.dll is loaded (used for language detection).
    pub has_system_runtime_dll: bool,
}

impl MapsInfo {
    /// Returns true if every signal has been found so the scan can stop early.
    /// In practice a single process is unlikely to have all signals (e.g. ddtrace
    /// is Python-specific while System.Runtime.dll is .NET-specific), so this is
    /// a theoretical upper bound rather than an expected hot path.
    fn all_found(&self) -> bool {
        self.has_apm_injector
            && self.has_gpu_nvidia
            && self.has_ddtrace
            && self.has_datadog_trace_dll
            && self.has_system_runtime_dll
    }
}

/// Reads /proc/[pid]/maps once and returns all extracted information.
pub fn read_maps_info(pid: i32) -> std::io::Result<MapsInfo> {
    let maps_path = root_path().join(pid.to_string()).join("maps");
    let file = File::open(maps_path)?;
    let reader = BufReader::new(file.take(MAPS_READ_LIMIT));

    Ok(read_maps_info_from_reader(reader))
}

/// Scans a maps reader and extracts all signals in a single pass.
///
/// Stops on the first IO error and returns whatever was found up to that point,
/// matching the behavior of the individual per-signal scanners this replaced.
pub fn read_maps_info_from_reader<R: BufRead>(reader: R) -> MapsInfo {
    let mut info = MapsInfo::default();

    for line in reader.split(b'\n').map_while(Result::ok) {
        if !info.has_gpu_nvidia && GPU_LIB_FINDERS.iter().any(|f| f.find(&line).is_some()) {
            info.has_gpu_nvidia = true;
        }

        if !info.has_ddtrace && DDTRACE_FINDER.find(&line).is_some() {
            info.has_ddtrace = true;
        }

        if !info.has_datadog_trace_dll && line.ends_with(b"Datadog.Trace.dll") {
            info.has_datadog_trace_dll = true;
        }

        if !info.has_system_runtime_dll && line.ends_with(b"/System.Runtime.dll") {
            info.has_system_runtime_dll = true;
        }

        if !info.has_apm_injector && is_injector_line(&line) {
            info.has_apm_injector = true;
        }

        if info.all_found() {
            break;
        }
    }

    info
}

/// Checks if a maps line contains the APM injector launcher.preload.so path.
///
/// Matches paths like:
///   /opt/datadog-packages/datadog-apm-inject/<version>/inject/launcher.preload.so
fn is_injector_line(line: &[u8]) -> bool {
    let Ok(line) = std::str::from_utf8(line) else {
        return false;
    };

    let Some(filename) = line.split_whitespace().last() else {
        return false;
    };

    let Some(after_prefix) = filename.strip_prefix("/opt/datadog-packages/datadog-apm-inject/")
    else {
        return false;
    };

    let Some(slash_pos) = after_prefix.find('/') else {
        return false;
    };

    // Non-empty version component
    if slash_pos == 0 {
        return false;
    }

    after_prefix
        .get(slash_pos..)
        .is_some_and(|remainder| remainder == "/inject/launcher.preload.so")
}

#[cfg(test)]
#[allow(clippy::expect_used)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn maps_info_from(maps_content: &str) -> MapsInfo {
        let reader = BufReader::new(maps_content.as_bytes());
        read_maps_info_from_reader(reader)
    }

    // ---- MapsInfo consolidated scan tests ----

    #[test]
    fn test_maps_info_empty() {
        let info = maps_info_from("");
        assert!(!info.has_apm_injector);
        assert!(!info.has_gpu_nvidia);
        assert!(!info.has_ddtrace);
        assert!(!info.has_datadog_trace_dll);
        assert!(!info.has_system_runtime_dll);
    }

    #[test]
    fn test_maps_info_apm_injector() {
        let info = maps_info_from(
            "ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920  /opt/datadog-packages/datadog-apm-inject/1.0.0/inject/launcher.preload.so\n",
        );
        assert!(info.has_apm_injector);
        assert!(!info.has_gpu_nvidia);
        assert!(!info.has_ddtrace);
    }

    #[test]
    fn test_maps_info_apm_injector_different_version() {
        let info = maps_info_from(
            "ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920  /opt/datadog-packages/datadog-apm-inject/2.5.3-beta/inject/launcher.preload.so\n",
        );
        assert!(info.has_apm_injector);
    }

    #[test]
    fn test_maps_info_apm_injector_similar_but_not_matching() {
        // Missing /inject/ in path
        let info = maps_info_from(
            "aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173  /opt/datadog-packages/datadog-apm-inject/1.0.0/launcher.preload.so\n",
        );
        assert!(!info.has_apm_injector);

        // Wrong suffix
        let info = maps_info_from(
            "aaaacd4ac000-aaaacd4b0000 r--p 000ec000 00:22 25173  /opt/datadog-packages/datadog-apm-inject/1.0.0/inject/launcher.so\n",
        );
        assert!(!info.has_apm_injector);

        // Wrong prefix
        let info = maps_info_from(
            "ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920  /opt/other-packages/datadog-apm-inject/1.0.0/inject/launcher.preload.so\n",
        );
        assert!(!info.has_apm_injector);
    }

    #[test]
    fn test_maps_info_gpu_nvidia() {
        let info = maps_info_from(
            "7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 456789 /usr/lib/libcuda.so.535\n",
        );
        assert!(info.has_gpu_nvidia);
        assert!(!info.has_apm_injector);
    }

    #[test]
    fn test_maps_info_ddtrace() {
        let info = maps_info_from(
            "7aef453fc000-7aef453ff000 rw-p 0004c000 fc:06 7895473  /home/foo/.local/lib/python3.10/site-packages/ddtrace/internal/_encoding.cpython-310-x86_64-linux-gnu.so\n",
        );
        assert!(info.has_ddtrace);
        assert!(!info.has_gpu_nvidia);
    }

    #[test]
    fn test_maps_info_ddtrace_not_matched_by_similar() {
        let info = maps_info_from(
            "79f6cd479000-79f6cd47a000 r-xp 00001000 fc:06 5507018  /home/foo/.local/lib/python3.10/site-packages/ddtrace_fake/md.cpython-310-x86_64-linux-gnu.so\n",
        );
        assert!(!info.has_ddtrace);
    }

    #[test]
    fn test_maps_info_datadog_trace_dll() {
        let info = maps_info_from(
            "785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267  /home/foo/hello/bin/release/net8.0/linux-x64/publish/Datadog.Trace.dll\n",
        );
        assert!(info.has_datadog_trace_dll);
        assert!(!info.has_system_runtime_dll);
    }

    #[test]
    fn test_maps_info_datadog_trace_dll_not_matched_by_similar() {
        let info = maps_info_from(
            "785c8ab24000-785c8ab2c000 r--s 00000000 fc:06 12762114  /home/foo/hello/publish/System.Diagnostics.StackTrace.dll\n",
        );
        assert!(!info.has_datadog_trace_dll);
    }

    #[test]
    fn test_maps_info_system_runtime_dll() {
        let info = maps_info_from(
            "7d97b4e85000-7d97b4e8e000 r--s 00000000 fc:04 1332665  /usr/lib/dotnet/shared/Microsoft.NETCore.App/8.0.8/System.Runtime.dll\n",
        );
        assert!(info.has_system_runtime_dll);
        assert!(!info.has_datadog_trace_dll);
    }

    #[test]
    fn test_maps_info_system_runtime_dll_partial_match() {
        let info = maps_info_from(
            "7d97b4e85000-7d97b4e8e000 r--s 00000000 fc:04 1332665  /usr/lib/dotnet/System.Runtime.dll.bak\n",
        );
        assert!(!info.has_system_runtime_dll);
    }

    #[test]
    fn test_maps_info_all_signals_combined() {
        let maps = "\
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 456789 /usr/lib/libcuda.so.535
7aef453fc000-7aef453ff000 rw-p 0004c000 fc:06 7895473  /home/foo/site-packages/ddtrace/internal/_encoding.so
785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267  /home/foo/publish/Datadog.Trace.dll
7d97b4e85000-7d97b4e8e000 r--s 00000000 fc:04 1332665  /usr/lib/dotnet/shared/System.Runtime.dll
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920  /opt/datadog-packages/datadog-apm-inject/1.0.0/inject/launcher.preload.so
";
        let info = maps_info_from(maps);
        assert!(info.has_apm_injector);
        assert!(info.has_gpu_nvidia);
        assert!(info.has_ddtrace);
        assert!(info.has_datadog_trace_dll);
        assert!(info.has_system_runtime_dll);
    }

    #[test]
    fn test_maps_info_no_matches_in_normal_process() {
        let maps = "\
55d8a0000000-55d8a0001000 r--p 00000000 08:01 123456 /usr/bin/python3.10
7f8e40000000-7f8e40021000 r--p 00000000 08:01 234567 /usr/lib/x86_64-linux-gnu/libc.so.6
7f8e48000000-7f8e48021000 r--p 00000000 08:01 345678 /usr/lib/x86_64-linux-gnu/libm.so.6
";
        let info = maps_info_from(maps);
        assert!(!info.has_apm_injector);
        assert!(!info.has_gpu_nvidia);
        assert!(!info.has_ddtrace);
        assert!(!info.has_datadog_trace_dll);
        assert!(!info.has_system_runtime_dll);
    }

    #[test]
    fn test_maps_info_stops_on_io_error() {
        use crate::test_utils::ErrorAfterReader;

        // No matches before error — should return default (all false)
        let content = b"7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libc.so.6\n";
        let reader = BufReader::new(ErrorAfterReader::new(&content[..]));
        let info = read_maps_info_from_reader(reader);
        assert!(!info.has_gpu_nvidia);

        // GPU match before error — should return partial result
        let content =
            b"7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535\n";
        let reader = BufReader::new(ErrorAfterReader::new(&content[..]));
        let info = read_maps_info_from_reader(reader);
        assert!(info.has_gpu_nvidia);

        // Empty, immediate error — should return default (all false)
        let reader = BufReader::new(ErrorAfterReader::new(&b""[..]));
        let info = read_maps_info_from_reader(reader);
        assert!(!info.has_gpu_nvidia);
    }

    #[test]
    fn test_maps_info_invalid_pid() {
        let temp_dir = TempDir::new().expect("Failed to create temp dir");
        temp_env::with_var("HOST_PROC", Some(temp_dir.path()), || {
            assert!(read_maps_info(999999).is_err());
        });
    }

    // ---- GPU detection tests ----

    #[test]
    fn test_gpu_without_gpu_libs() {
        let info = maps_info_from(
            "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libc.so.6
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libm.so.6
7f8e4c400000-7f8e4c500000 r--p 00000000 08:01 123456 /usr/lib/libpthread.so.0
",
        );
        assert!(!info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_multiple_gpu_libs() {
        let info = maps_info_from(
            "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libcudart.so.11.0
7f8e4c400000-7f8e4c500000 r--p 00000000 08:01 123456 /usr/lib/libcudnn.so.8
7f8e4c500000-7f8e4c600000 r--p 00000000 08:01 123456 /usr/lib/libnccl.so.2
",
        );
        assert!(info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_realistic_maps() {
        let info = maps_info_from(
            "
55d8a0000000-55d8a0001000 r--p 00000000 08:01 123456 /usr/bin/python3.10
55d8a0001000-55d8a0002000 r-xp 00001000 08:01 123456 /usr/bin/python3.10
55d8a0002000-55d8a0003000 r--p 00002000 08:01 123456 /usr/bin/python3.10
7f8e40000000-7f8e40021000 r--p 00000000 08:01 234567 /usr/lib/x86_64-linux-gnu/libc.so.6
7f8e40021000-7f8e401a0000 r-xp 00021000 08:01 234567 /usr/lib/x86_64-linux-gnu/libc.so.6
7f8e401a0000-7f8e401f8000 r--p 001a0000 08:01 234567 /usr/lib/x86_64-linux-gnu/libc.so.6
7f8e48000000-7f8e48021000 r--p 00000000 08:01 345678 /usr/lib/x86_64-linux-gnu/libm.so.6
7f8e48021000-7f8e480a0000 r-xp 00021000 08:01 345678 /usr/lib/x86_64-linux-gnu/libm.so.6
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 456789 /usr/lib/x86_64-linux-gnu/libcuda.so.535.129.03
7f8e4c021000-7f8e4e400000 r-xp 00021000 08:01 456789 /usr/lib/x86_64-linux-gnu/libcuda.so.535.129.03
7f8e50000000-7f8e50001000 r--p 00000000 08:01 567890 /usr/lib/x86_64-linux-gnu/libpthread.so.0
",
        );
        assert!(info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_case_sensitive() {
        let info = maps_info_from(
            "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/LIBCUDA.SO
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/LIBCUDART.SO
",
        );
        assert!(!info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_with_whitespace() {
        let info = maps_info_from(
            "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456    /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456    /usr/lib/libc.so.6
",
        );
        assert!(info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_partial_match() {
        let info = maps_info_from(
            "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/cuda/lib64/libcublas.so.11.10.3.66
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /opt/cuda/extras/CUPTI/lib64/libnvrtc.so.11.2.152
",
        );
        assert!(info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_with_newlines() {
        let info = maps_info_from(
            "\n\n7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so\n\n",
        );
        assert!(info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_large_maps_file() {
        let mut maps_content = String::new();
        for i in 0..1000 {
            maps_content.push_str(&format!(
                "7f8e{:08x}-7f8e{:08x} r--p 00000000 08:01 {} /usr/lib/libc.so.6\n",
                i * 0x1000,
                (i + 1) * 0x1000,
                i
            ));
        }
        maps_content.push_str(
            "7f8eff000000-7f8eff021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535\n",
        );

        assert!(maps_info_from(&maps_content).has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_library_at_start_large_maps_file() {
        let mut maps_content = String::new();
        maps_content.push_str(
            "7f8e00000000-7f8e00021000 r--p 00000000 08:01 000001 /usr/lib/libcuda.so.535\n",
        );
        for i in 1..1000 {
            maps_content.push_str(&format!(
                "7f8e{:08x}-7f8e{:08x} r--p 00000000 08:01 {} /usr/lib/libc.so.6\n",
                i * 0x1000,
                (i + 1) * 0x1000,
                i
            ));
        }

        assert!(maps_info_from(&maps_content).has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_no_newline_at_end() {
        let info = maps_info_from(
            "7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535",
        );
        assert!(info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_with_libnvidia_ml() {
        let info = maps_info_from(
            "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libnvidia-ml.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libnvidia-ml.so.535
",
        );
        assert!(info.has_gpu_nvidia);
    }

    #[test]
    fn test_gpu_other_nvidia_libs_not_detected() {
        let non_matching_libs = [
            "/usr/lib/libnvidia-glcore.so.535",
            "/usr/lib/libnvidia-ptxjitcompiler.so.535",
            "/usr/lib/libnvidia-cfg.so.535",
            "/usr/lib/libnvidia-compiler.so.535",
            "/usr/lib/libnvidia-eglcore.so.535",
        ];

        for lib in non_matching_libs.iter() {
            let maps_content = format!(
                "7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 {}\n",
                lib
            );
            assert!(
                !maps_info_from(&maps_content).has_gpu_nvidia,
                "Should NOT detect library: {}",
                lib
            );
        }
    }
}
