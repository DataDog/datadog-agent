// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use nix::unistd::{Uid, User};
use std::fs;
use std::io::{BufRead, BufReader};

pub(super) fn lookup_runtime_user(pid: u32) -> Result<String> {
    let status_path = format!("/proc/{pid}/status");
    let file = fs::File::open(&status_path).context("open process status")?;
    let uid = parse_effective_uid(&file).context("parse process uid")?;
    User::from_uid(Uid::from_raw(uid))
        .context("getpwuid")?
        .map(|u| u.name)
        .context("no passwd entry for uid")
}

/// Parse the effective UID from a `/proc/<pid>/status` `Uid:` line.
///
/// Format: `Uid: <real> <effective> <saved> <fs>`.
fn parse_effective_uid(file: &fs::File) -> Result<u32> {
    parse_effective_uid_from_reader(&mut BufReader::new(file))
}

fn parse_effective_uid_from_reader<R: BufRead>(reader: &mut R) -> Result<u32> {
    for line in reader.lines() {
        let line = line?;
        if let Some(rest) = line.strip_prefix("Uid:") {
            let effective = rest
                .split_whitespace()
                .nth(1)
                .context("parse effective uid from Uid line")?;
            return effective.parse().context("parse uid");
        }
    }
    bail!("Uid not found in process status")
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    #[test]
    fn parse_effective_uid_reads_second_field() {
        let status = "Name:\tsleep\nUid:\t1000\t0\t0\t0\n";
        let mut reader = Cursor::new(status.as_bytes());
        assert_eq!(
            parse_effective_uid_from_reader(&mut reader).expect("effective uid"),
            0
        );
    }

    #[test]
    fn parse_effective_uid_matches_when_real_and_effective_agree() {
        let status = "Uid: 1000 1000 1000 1000\n";
        let mut reader = Cursor::new(status.as_bytes());
        assert_eq!(
            parse_effective_uid_from_reader(&mut reader).expect("effective uid"),
            1000
        );
    }
}
