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
static GPU_NVIDIA_LIB_FINDERS: LazyLock<[Finder<'static>; 7]> = LazyLock::new(|| {
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

// Pre-built finders for AMD ROCm GPU compute libraries
static GPU_AMD_LIB_FINDERS: LazyLock<[Finder<'static>; 6]> = LazyLock::new(|| {
    [
        Finder::new(b"libamdhip64.so"),
        Finder::new(b"librocblas.so"),
        Finder::new(b"libMIOpen.so"),
        Finder::new(b"librocm_smi64.so"),
        Finder::new(b"librccl.so"),
        Finder::new(b"libhipblas.so"),
    ]
});

// Pre-built finders for Intel GPU compute libraries (Level Zero)
static GPU_INTEL_LIB_FINDERS: LazyLock<[Finder<'static>; 1]> =
    LazyLock::new(|| [Finder::new(b"libze_intel_gpu.so")]);

/// Scans a reader for any GPU library fingerprint in a single pass over its lines.
/// Shared by the public API and test helpers to avoid logic duplication.
fn check_for_any_gpu_libraries<R: BufRead>(reader: R) -> bool {
    reader.split(b'\n').map_while(Result::ok).any(|line| {
        GPU_NVIDIA_LIB_FINDERS
            .iter()
            .any(|f| f.find(&line).is_some())
            || GPU_AMD_LIB_FINDERS.iter().any(|f| f.find(&line).is_some())
            || GPU_INTEL_LIB_FINDERS
                .iter()
                .any(|f| f.find(&line).is_some())
    })
}

/// Detects if a process is using any GPU libraries by scanning /proc/[pid]/maps once.
/// Checks NVIDIA, AMD ROCm, and Intel GPU compute libraries in a single pass.
pub fn has_any_gpu_libraries(pid: i32) -> bool {
    let Ok(reader) = get_reader_for_pid(pid) else {
        return false;
    };
    check_for_any_gpu_libraries(reader)
}

#[cfg(test)]
#[allow(clippy::expect_used)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn check_for_gpu_libraries<R: BufRead>(reader: R, finders: &[Finder<'static>]) -> bool {
        reader
            .split(b'\n')
            .map_while(|line_result| line_result.ok())
            .any(|line| finders.iter().any(|f| f.find(&line).is_some()))
    }

    fn test_nvidia_with_mock_maps(maps_content: &str) -> bool {
        let reader = BufReader::new(maps_content.as_bytes());
        check_for_gpu_libraries(reader, &*GPU_NVIDIA_LIB_FINDERS)
    }

    fn test_amd_with_mock_maps(maps_content: &str) -> bool {
        let reader = BufReader::new(maps_content.as_bytes());
        check_for_gpu_libraries(reader, &*GPU_AMD_LIB_FINDERS)
    }

    fn test_intel_with_mock_maps(maps_content: &str) -> bool {
        let reader = BufReader::new(maps_content.as_bytes());
        check_for_gpu_libraries(reader, &*GPU_INTEL_LIB_FINDERS)
    }

    fn test_has_any_gpu_with_mock_maps(maps_content: &str) -> bool {
        let reader = BufReader::new(maps_content.as_bytes());
        check_for_any_gpu_libraries(reader)
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_without_gpu_libs() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libc.so.6
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libm.so.6
7f8e4c400000-7f8e4c500000 r--p 00000000 08:01 123456 /usr/lib/libpthread.so.0
";
        assert!(!test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_multiple_gpu_libs() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libcudart.so.11.0
7f8e4c400000-7f8e4c500000 r--p 00000000 08:01 123456 /usr/lib/libcudnn.so.8
7f8e4c500000-7f8e4c600000 r--p 00000000 08:01 123456 /usr/lib/libnccl.so.2
";
        assert!(test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_empty_maps() {
        let maps_content = "";
        assert!(!test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_any_gpu_libraries_invalid_pid() {
        // Test with PID that doesn't have a maps file
        let temp_dir = TempDir::new().expect("Failed to create temp dir");
        temp_env::with_var("HOST_PROC", Some(temp_dir.path()), || {
            assert!(!has_any_gpu_libraries(999999));
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
        assert!(test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_case_sensitive() {
        // Library names in uppercase (should not match on Linux)
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/LIBCUDA.SO
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/LIBCUDART.SO
";
        assert!(!test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_whitespace() {
        // Extra whitespace between columns in the maps line format should not prevent detection
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456    /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456    /usr/lib/libc.so.6
";
        assert!(test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_partial_match() {
        // Test that substring matching works for versioned libraries
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/cuda/lib64/libcublas.so.11.10.3.66
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /opt/cuda/extras/CUPTI/lib64/libnvrtc.so.11.2.152
";
        assert!(test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_newlines() {
        let maps_content =
            "\n\n7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so\n\n";
        assert!(test_nvidia_with_mock_maps(maps_content));
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

        assert!(test_nvidia_with_mock_maps(&maps_content));
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

        assert!(test_nvidia_with_mock_maps(&maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_no_newline_at_end() {
        let maps_content =
            "7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535";
        assert!(test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_libnvidia_ml() {
        // Test that libnvidia-ml.so is detected
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libnvidia-ml.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libnvidia-ml.so.535
";
        assert!(test_nvidia_with_mock_maps(maps_content));
    }

    /// Verify that check_for_gpu_libraries terminates on I/O error instead
    /// of spinning forever (regression test for ESRCH infinite loop).
    #[test]
    fn test_check_for_gpu_libraries_terminates_on_io_error() {
        use crate::test_utils::ErrorAfterReader;

        // No GPU libraries before the error — should return false, not hang.
        let content = b"7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libc.so.6\n";
        let reader = BufReader::new(ErrorAfterReader::new(&content[..]));
        assert!(!check_for_any_gpu_libraries(reader));

        // GPU library present before error — should find it and return true.
        let content =
            b"7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535\n";
        let reader = BufReader::new(ErrorAfterReader::new(&content[..]));
        assert!(check_for_any_gpu_libraries(reader));

        // Empty content, immediate error — should return false, not hang.
        let reader = BufReader::new(ErrorAfterReader::new(&b""[..]));
        assert!(!check_for_any_gpu_libraries(reader));
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
                !test_nvidia_with_mock_maps(&maps_content),
                "Should NOT detect library: {}",
                lib
            );
        }
    }

    #[test]
    fn test_has_gpu_amd_libraries_detected() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/rocm/lib/libamdhip64.so.5
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /opt/rocm/lib/librocblas.so.3
";
        assert!(test_amd_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_amd_libraries_miopen() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/rocm/lib/libMIOpen.so.1
";
        assert!(test_amd_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_amd_libraries_rocm_smi() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/rocm/lib/librocm_smi64.so.5
";
        assert!(test_amd_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_amd_libraries_not_detected_without_amd_libs() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libc.so.6
";
        assert!(!test_amd_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_amd_libraries_rccl() {
        // librccl.so is the AMD distributed-training comms library (ROCm equivalent of libnccl.so)
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/rocm/lib/librccl.so.1
";
        assert!(test_amd_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_amd_libraries_hipblas() {
        // libhipblas.so is the HIP BLAS library, used in ROCm ML workloads
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/rocm/lib/libhipblas.so.2
";
        assert!(test_amd_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_intel_libraries_level_zero() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/x86_64-linux-gnu/libze_intel_gpu.so.1
";
        assert!(test_intel_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_gpu_intel_libraries_not_detected_without_intel_libs() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libamdhip64.so.5
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libc.so.6
";
        assert!(!test_intel_with_mock_maps(maps_content));
    }

    #[test]
    fn test_amd_libs_not_detected_by_nvidia_finder() {
        // AMD libraries must not trigger NVIDIA detection
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/rocm/lib/libamdhip64.so.5
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /opt/rocm/lib/librocblas.so.3
";
        assert!(!test_nvidia_with_mock_maps(maps_content));
    }

    #[test]
    fn test_intel_libs_not_detected_by_nvidia_finder() {
        // Intel libraries must not trigger NVIDIA detection
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libze_intel_gpu.so.1
";
        assert!(!test_nvidia_with_mock_maps(maps_content));
    }

    // --- Direct tests for the public has_any_gpu_libraries function ---

    #[test]
    fn test_has_any_gpu_libraries_nvidia_only() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libc.so.6
";
        assert!(test_has_any_gpu_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_any_gpu_libraries_amd_only() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /opt/rocm/lib/libamdhip64.so.5
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libc.so.6
";
        assert!(test_has_any_gpu_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_any_gpu_libraries_intel_only() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libze_intel_gpu.so.1
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libc.so.6
";
        assert!(test_has_any_gpu_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_any_gpu_libraries_mixed_nvidia_and_amd() {
        // A process using both NVIDIA and AMD libraries (e.g. heterogeneous node) must be detected
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /opt/rocm/lib/libamdhip64.so.5
";
        assert!(test_has_any_gpu_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_any_gpu_libraries_mixed_nvidia_and_intel() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libze_intel_gpu.so.1
";
        assert!(test_has_any_gpu_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_any_gpu_libraries_no_gpu_libs() {
        // A process with only standard system libraries must not be detected
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib/libc.so.6
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib/libm.so.6
7f8e4c400000-7f8e4c500000 r--p 00000000 08:01 123456 /usr/lib/libpthread.so.0
";
        assert!(!test_has_any_gpu_with_mock_maps(maps_content));
    }

    #[test]
    fn test_has_any_gpu_libraries_empty_maps() {
        assert!(!test_has_any_gpu_with_mock_maps(""));
    }
}
