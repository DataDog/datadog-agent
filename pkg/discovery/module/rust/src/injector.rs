// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::io::BufRead;

use crate::procfs;

pub fn is_apm_injector_in_process_maps(pid: i32) -> bool {
    log::debug!("Checking injector status for {}", pid);
    let Ok(maps_reader) = procfs::maps::get_reader_for_pid(pid) else {
        return false;
    };

    is_injector_in_maps(maps_reader)
}

fn is_injector_in_maps<T>(maps_reader: T) -> bool
where
    T: BufRead,
{
    maps_reader.lines().any(|line| {
        let Ok(line) = line else {
            return false;
        };

        let Some(filename) = line.split_whitespace().last() else {
            return false;
        };

        // Strip prefix
        let Some(after_prefix) = filename.strip_prefix("/opt/datadog-packages/datadog-apm-inject/")
        else {
            return false;
        };

        // Find the next slash (end of version/middle part)
        let Some(slash_pos) = after_prefix.find('/') else {
            return false;
        };

        // Check that there's at least one char before the slash (non-empty middle)
        if slash_pos == 0 {
            return false;
        }

        // Check that remaining part matches exactly
        after_prefix
            .get(slash_pos..)
            .is_some_and(|remainder| remainder == "/inject/launcher.preload.so")
    })
}

#[cfg(test)]
mod tests {
    use std::io::Cursor;

    use super::*;

    #[test]
    fn test_empty_maps() {
        let maps = "";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!is_injector_in_maps(reader));
    }

    #[test]
    fn test_no_injector_in_maps() {
        let maps =
            "aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /usr/bin/bash
aaaacd4ac000-aaaacd4b0000 r--p 000ec000 00:22 25173                      /usr/bin/bash
aaaacd4b0000-aaaacd4b4000 rw-p 000f0000 00:22 25173                      /usr/bin/bash
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /usr/lib64/libc.so.6
ffffb74ec000-ffffb74fd000 ---p 0018c000 00:22 13920                      /usr/lib64/libc.so.6";

        let reader = Cursor::new(maps.as_bytes());
        assert!(!is_injector_in_maps(reader));
    }

    #[test]
    fn test_injector_present() {
        let maps = "aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /usr/bin/bash
aaaacd4ac000-aaaacd4b0000 r--p 000ec000 00:22 25173                      /usr/bin/bash
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /opt/datadog-packages/datadog-apm-inject/1.0.0/inject/launcher.preload.so
ffffb74ec000-ffffb74fd000 ---p 0018c000 00:22 13920                      /usr/lib64/libc.so.6";

        let reader = Cursor::new(maps.as_bytes());
        assert!(is_injector_in_maps(reader));
    }

    #[test]
    fn test_injector_with_different_version() {
        let maps = "aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /usr/bin/bash
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /opt/datadog-packages/datadog-apm-inject/2.5.3-beta/inject/launcher.preload.so";

        let reader = Cursor::new(maps.as_bytes());
        assert!(is_injector_in_maps(reader));
    }

    #[test]
    fn test_similar_but_not_matching_paths() {
        let maps = "aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /opt/datadog-packages/datadog-apm-inject/1.0.0/launcher.preload.so
aaaacd4ac000-aaaacd4b0000 r--p 000ec000 00:22 25173                      /opt/datadog-packages/datadog-apm-inject/1.0.0/inject/launcher.so
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /opt/other-packages/datadog-apm-inject/1.0.0/inject/launcher.preload.so";

        let reader = Cursor::new(maps.as_bytes());
        assert!(!is_injector_in_maps(reader));
    }
}
