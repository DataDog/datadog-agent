// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Context;
use windows_sys::Win32::Security::TOKEN_QUERY;

use super::super::local_agent_account::{AgentAccount, agent_account_from_well_known_sid};
use super::super::token_identity::{
    current_process_account_display, open_current_process_token, token_user_is_local_system,
    token_user_sid_bytes,
};
use super::credential::SpawnCredential;

pub(super) fn agent_profile_credential() -> anyhow::Result<SpawnCredential> {
    let token = open_current_process_token(TOKEN_QUERY)
        .context("open supervisor token for test harness spawn credential")?;
    let account = if token_user_is_local_system(token.as_handle())
        .context("classify supervisor token for test harness")?
    {
        AgentAccount::LocalSystem
    } else {
        let sid = token_user_sid_bytes(token.as_handle())
            .context("read supervisor token SID for test harness")?;
        agent_account_from_well_known_sid(&sid).unwrap_or_else(|| {
            password_logon_from_display(
                current_process_account_display()
                    .context("lookup supervisor account name for test harness")?,
            )
        })
    };

    Ok(SpawnCredential::from_account(account))
}

fn password_logon_from_display(display: String) -> AgentAccount {
    let (domain, user) = match display.split_once('\\') {
        Some((domain, user)) if domain != "." => (domain.to_string(), user.to_string()),
        Some((_, user)) => (String::new(), user.to_string()),
        None => (String::new(), display),
    };
    AgentAccount::PasswordLogon {
        domain,
        user,
        password: String::new(),
    }
}
