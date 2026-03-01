// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::fs::File;
use std::io::{BufRead, BufReader, Read, Take};
use std::sync::LazyLock;

use memchr::memmem::Finder;

use super::root_path;

const MAPS_READ_LIMIT: u64 = 4 * 1024 * 1024 * 1024; // 4GiB

pub fn get_reader_for_pid(pid: i32) -> Result<BufReader<Take<File>>, std::io::Error> {
    let maps_path = root_path().join(pid.to_string()).join("maps");
    let file = File::open(maps_path)?;
    Ok(BufReader::new(file.take(MAPS_READ_LIMIT)))
}

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

/// Internal function to check for GPU libraries given a reader
fn check_for_gpu_libraries<R: BufRead>(reader: R) -> bool {
    reader
        .split(b'\n')
        .filter_map(|line_result| line_result.ok())
        .any(|line| GPU_LIB_FINDERS.iter().any(|f| f.find(&line).is_some()))
}

/// Detects if a process is using NVIDIA GPU libraries by checking /proc/[pid]/maps
pub fn has_gpu_nvidia_libraries(pid: i32) -> bool {
    let Ok(reader) = get_reader_for_pid(pid) else {
        return false;
    };

    check_for_gpu_libraries(reader)
}

#[cfg(test)]
#[allow(clippy::expect_used)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn test_with_mock_maps(maps_content: &str) -> bool {
        let reader = BufReader::new(maps_content.as_bytes());
        check_for_gpu_libraries(reader)
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_without_gpu_libs() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libc.so.6
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libm.so.6
7f8e4c400000-7f8e4c500000 r--p 00000000 08:01 123456 /usr/lib/libpthread.so.0
";
        assert!(!test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_multiple_gpu_libs() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libcudart.so.11.0
7f8e4c400000-7f8e4c500000 r--p 00000000 08:01 123456 /usr/lib/libcudnn.so.8
7f8e4c500000-7f8e4c600000 r--p 00000000 08:01 123456 /usr/lib/libnccl.so.2
";
        assert!(test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_empty_maps() {
        let maps_content = "";
        assert!(!test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_invalid_pid() {
        // Test with PID that doesn't have a maps file
        let temp_dir = TempDir::new().expect("Failed to create temp dir");
        temp_env::with_var("HOST_PROC", Some(temp_dir.path()), || {
            assert!(!has_gpu_nvidia_libraries(999999));
        });
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_realistic_maps() {
        // Realistic /proc/pid/maps content with many entries
        let maps_content = "
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
";
        assert!(test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_case_sensitive() {
        // Library names in uppercase (should not match on Linux)
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/LIBCUDA.SO
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/LIBCUDART.SO
";
        assert!(!test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_whitespace() {
        // Extra whitespace between columns in the maps line format should not prevent detection
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456    /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456    /usr/lib/libc.so.6
";
        assert!(test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_partial_match() {
        // Test that substring matching works for versioned libraries
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/cuda/lib64/libcublas.so.11.10.3.66
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /opt/cuda/extras/CUPTI/lib64/libnvrtc.so.11.2.152
";
        assert!(test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_newlines() {
        let maps_content =
            "\n\n7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so\n\n";
        assert!(test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_large_maps_file() {
        // GPU library at the end: verifies the scanner handles large files correctly (worst case)
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

        assert!(test_with_mock_maps(&maps_content));
    }

    #[test]
    fn test_gpu_library_at_start_large_maps_file() {
        // Verifies that a GPU library present at the beginning of the maps file is correctly detected even when followed by many non-GPU entries.
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

        assert!(test_with_mock_maps(&maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_no_newline_at_end() {
        let maps_content =
            "7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535";
        assert!(test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_libnvidia_ml() {
        // Test that libnvidia-ml.so is detected
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libnvidia-ml.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libnvidia-ml.so.535
";
        assert!(test_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_other_nvidia_libs_not_detected() {
        // Test that other libnvidia-* libraries are NOT detected (only libnvidia-ml.so is)
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
                !test_with_mock_maps(&maps_content),
                "Should NOT detect library: {}",
                lib
            );
        }
    }
}
