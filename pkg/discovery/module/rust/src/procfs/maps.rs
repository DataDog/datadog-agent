// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::fs::File;
use std::io::{BufRead, BufReader, Read, Take};

use super::root_path;

const MAPS_READ_LIMIT: u64 = 4 * 1024 * 1024 * 1024; // 4GiB

pub fn get_reader_for_pid(pid: i32) -> Result<BufReader<Take<File>>, std::io::Error> {
    let maps_path = root_path().join(pid.to_string()).join("maps");
    let file = File::open(maps_path)?;
    Ok(BufReader::new(file.take(MAPS_READ_LIMIT)))
}

// List of NVIDIA-specific GPU libraries to detect as raw bytes
const GPU_LIBS: &[&[u8]] = &[
    b"libcuda.so",
    b"libcudart.so",
    b"libnvidia-ml.so",
    b"libnvrtc.so",
    b"libcudnn.so",
    b"libcublas.so",
    b"libnccl.so",
];

/// Internal function to check for GPU libraries given a reader
fn check_for_gpu_libraries<R: BufRead>(reader: R) -> bool {
    // Read the maps file line by line and check for any of the GPU libraries
    reader
        .split(b'\n')
        .filter_map(|line_result| line_result.ok())
        .any(|line| {
            GPU_LIBS
                .iter()
                .any(|&lib| line.windows(lib.len()).any(|window| window == lib))
        })
}

// NVIDIA device patterns we are testing here:
// - /dev/nvidia<N>         (e.g. /dev/nvidia0)
// - /dev/nvidiactl         (control device)
// - /dev/nvidia-uvm        (Unified Virtual Memory)
// - /dev/nvidia-uvm-tools  (UVM tooling)
// - /dev/nvidia-modeset    (modeset device)
fn is_gpu_device(device_path: &str) -> bool {
    if let Some(s) = device_path.strip_prefix("/dev/nvidia") {
        return s == "ctl"
            || s == "-uvm"
            || s == "-uvm-tools"
            || s == "-modeset"
            || (!s.is_empty() && s.bytes().all(|b| b.is_ascii_digit())); // nvidia0, nvidia1, ...
    }
    false
}

/// Checks if a process has GPU devices open by examining /proc/[pid]/fd
fn check_for_gpu_devices(pid: i32) -> bool {
    let fd_path = root_path().join(pid.to_string()).join("fd");

    let Ok(entries) = std::fs::read_dir(fd_path) else {
        return false;
    };

    entries.filter_map(|entry| entry.ok()).any(|entry| {
        std::fs::read_link(entry.path())
            .ok()
            .and_then(|target| target.to_str().map(|s| s.to_string()))
            .map(|target| is_gpu_device(&target))
            .unwrap_or(false)
    })
}

/// Detects if a process is using NVIDIA GPU by checking both devices (first check) and libraries (if no devices found).
pub fn has_gpu_nvidia(pid: i32) -> bool {
    // Fast path: Check devices
    if check_for_gpu_devices(pid) {
        return true;
    }

    // Slow path: Check libraries only if no device found
    let Ok(reader) = get_reader_for_pid(pid) else {
        return false;
    };

    check_for_gpu_libraries(reader)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    /// Helper to test GPU detection with mocked maps content
    /// Creates a reader from the mock content and uses the real detection logic
    fn test_with_mock_maps(maps_content: &str) -> bool {
        let reader = BufReader::new(maps_content.as_bytes());
        check_for_gpu_libraries(reader)
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_libcuda() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib64/libcuda.so.535.129.03
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib64/libcuda.so.535.129.03
";
        assert!(test_with_mock_maps(maps_content));
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
        let Ok(temp_dir) = TempDir::new() else {
            return;
        };
        temp_env::with_var("HOST_PROC", Some(temp_dir.path()), || {
            assert!(!has_gpu_nvidia(999999));
        });
    }

    #[test]
    fn test_has_gpu_nvidia_libraries_with_version_numbers() {
        let maps_content = "
7f8e4c000000-7f8e4c021000 r--p 00000000 08:01 123456 /usr/lib64/libnvidia-ml.so.535.129.03
7f8e4c021000-7f8e4c400000 r-xp 00021000 08:01 123456 /usr/lib64/libcudart.so.11.8.89
";
        assert!(test_with_mock_maps(maps_content));
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
        // Simulate a large maps file with 1000 entries
        let mut maps_content = String::new();
        for i in 0..1000 {
            maps_content.push_str(&format!(
                "7f8e{:08x}-7f8e{:08x} r--p 00000000 08:01 {} /usr/lib/libc.so.6\n",
                i * 0x1000,
                (i + 1) * 0x1000,
                i
            ));
        }
        // Add GPU library at the end
        maps_content.push_str(
            "7f8eff000000-7f8eff021000 r--p 00000000 08:01 123456 /usr/lib/libcuda.so.535\n",
        );

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

    #[test]
    fn test_is_gpu_device_nvidia_numbered() {
        // NVIDIA GPU devices with numbers
        assert!(is_gpu_device("/dev/nvidia0"));
        assert!(is_gpu_device("/dev/nvidia1"));
        assert!(is_gpu_device("/dev/nvidia2"));
        assert!(is_gpu_device("/dev/nvidia10"));
        assert!(is_gpu_device("/dev/nvidia123"));
    }

    #[test]
    fn test_is_gpu_device_nvidia_special() {
        // NVIDIA special devices
        assert!(is_gpu_device("/dev/nvidiactl"));
        assert!(is_gpu_device("/dev/nvidia-uvm"));
        assert!(is_gpu_device("/dev/nvidia-uvm-tools"));
        assert!(is_gpu_device("/dev/nvidia-modeset"));
    }

    #[test]
    fn test_is_gpu_device_invalid_nvidia_names() {
        // Invalid NVIDIA device names (stricter validation now)
        assert!(!is_gpu_device("/dev/nvidia"));
        assert!(!is_gpu_device("/dev/nvidia-foo"));
        assert!(!is_gpu_device("/dev/nvidiaXYZ"));
        assert!(!is_gpu_device("/dev/nvidia0abc"));
        assert!(!is_gpu_device("/dev/nvidia-"));
        assert!(!is_gpu_device("/dev/nvidiactl2"));
        assert!(!is_gpu_device("/dev/nvidia0-uvm"));
    }

    #[test]
    fn test_is_gpu_device_edge_cases() {
        // Path variations
        assert!(!is_gpu_device("/dev/nvi"));
        assert!(!is_gpu_device("/home/nvidia0"));
        assert!(!is_gpu_device("/dev/dri/card0"));
        assert!(!is_gpu_device("nvidia0"));
        assert!(!is_gpu_device("/dev/nvidia 0"));
        assert!(!is_gpu_device("/dev/NVIDIA0"));
    }

    #[test]
    fn test_check_gpu_devices_nonexistent_pid() {
        // PID that definitely doesn't exist
        assert!(!check_for_gpu_devices(999999));
    }

    #[test]
    fn test_hybrid_detection_nonexistent_process() {
        // Should return false for non-existent process
        assert!(!has_gpu_nvidia(999999));
    }
}
