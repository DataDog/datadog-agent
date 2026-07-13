// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
use libc::{PROC_PIDTBSDINFO, c_int, proc_bsdinfo, proc_pidinfo};
use nix::unistd::{Uid, User};

pub(super) fn lookup_runtime_user(pid: u32) -> Result<String> {
    let mut info = unsafe { std::mem::zeroed::<proc_bsdinfo>() };
    let size = std::mem::size_of::<proc_bsdinfo>() as c_int;
    let result = unsafe {
        proc_pidinfo(
            pid as c_int,
            PROC_PIDTBSDINFO,
            0,
            (&raw mut info).cast(),
            size,
        )
    };
    if result != size {
        bail!("proc_pidinfo: {}", std::io::Error::last_os_error());
    }

    User::from_uid(Uid::from_raw(info.pbi_uid))
        .context("getpwuid")?
        .map(|u| u.name)
        .context("no passwd entry for uid")
}
