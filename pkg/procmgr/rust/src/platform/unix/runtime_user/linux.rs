// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use nix::unistd::{Uid, User};
use std::fs;
use std::io::BufRead;

pub(super) fn lookup_runtime_user(pid: u32) -> Result<String> {
    let status_path = format!("/proc/{pid}/status");
    let file = fs::File::open(&status_path).context("open process status")?;
    let uid = parse_real_uid(&file).context("parse process uid")?;
    User::from_uid(Uid::from_raw(uid))
        .context("getpwuid")?
        .map(|u| u.name)
        .context("no passwd entry for uid")
}

fn parse_real_uid(file: &fs::File) -> Result<u32> {
    let reader = std::io::BufReader::new(file);
    for line in reader.lines() {
        let line = line?;
        if let Some(rest) = line.strip_prefix("Uid:") {
            let first = rest.split_whitespace().next().context("parse Uid line")?;
            return first.parse().context("parse uid");
        }
    }
    bail!("Uid not found in process status")
}
