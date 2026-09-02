// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Context;
use windows_sys::Win32::Security::TOKEN_QUERY;

use super::super::token_identity::{
    current_process_account_display, open_current_process_token, token_user_is_local_system,
};
use super::credential::SpawnCredential;

pub(super) fn agent_profile_credential() -> anyhow::Result<SpawnCredential> {
    let token = open_current_process_token(TOKEN_QUERY)
        .context("open supervisor token for test harness spawn credential")?;
    let require_local_system = token_user_is_local_system(token.as_handle())
        .context("classify supervisor token for test harness")?;

    let display_name = if require_local_system {
        "NT AUTHORITY\\SYSTEM".to_string()
    } else {
        current_process_account_display()
            .context("lookup supervisor account name for test harness")?
    };

    Ok(SpawnCredential::InheritSupervisor {
        display_name,
        require_local_system,
    })
}
