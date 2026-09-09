// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};

use super::super::local_agent_account::AgentAccount;
#[cfg(not(test))]
use super::super::local_agent_account::resolve_agent_account;
use super::super::token_identity::open_current_process_token;
use super::win32::duplicate_primary_token;

#[derive(Clone, Debug)]
pub(crate) struct SpawnCredential {
    account: AgentAccount,
    reuses_supervisor_token: bool,
}

impl SpawnCredential {
    pub(crate) fn resolve_agent() -> Result<Self> {
        #[cfg(test)]
        {
            super::test_harness::agent_profile_credential()
        }
        #[cfg(not(test))]
        {
            Self::new(resolve_agent_account()?)
        }
    }

    fn new(account: AgentAccount) -> Result<Self> {
        let reuses_supervisor_token = account
            .reuses_supervisor_token()
            .context("compare spawn account to supervisor token")?;
        Ok(Self {
            account,
            reuses_supervisor_token,
        })
    }

    pub(crate) fn display_name(&self) -> String {
        self.account.display_name()
    }

    pub(crate) fn account(&self) -> &AgentAccount {
        &self.account
    }

    pub(crate) fn reuses_supervisor_token(&self) -> bool {
        self.reuses_supervisor_token
    }

    pub(crate) fn duplicate_primary_token(&self, process_name: &str) -> Result<HANDLE> {
        if !self.reuses_supervisor_token() {
            anyhow::bail!(
                "[{process_name}] duplicate_primary_token requires supervisor-token reuse"
            );
        }
        let supervisor_token =
            open_current_process_token(TOKEN_QUERY | TOKEN_DUPLICATE).map_err(|e| {
                anyhow::anyhow!(
                    "[{process_name}] OpenProcessToken(GetCurrentProcess()) failed: {e}"
                )
            })?;
        duplicate_primary_token(process_name, supervisor_token.as_handle())
    }
}

#[cfg(test)]
impl SpawnCredential {
    pub(super) fn from_account(account: AgentAccount) -> Result<Self> {
        Self::new(account)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn agent_profile_in_unit_tests_never_logons_installer_account() {
        let credential = SpawnCredential::resolve_agent().expect("resolve agent profile in tests");
        match credential.account() {
            AgentAccount::LocalSystem
            | AgentAccount::LocalService
            | AgentAccount::NetworkService
            | AgentAccount::SupervisorAccount { .. } => {}
            AgentAccount::PasswordLogon { .. } => {
                panic!(
                    "test harness must inherit the supervisor token, not SCM installer password: \
                     {credential:?}"
                );
            }
        }
    }

    #[test]
    fn logon_credential_display_name_matches_account() {
        let credential =
            SpawnCredential::from_account(AgentAccount::LocalSystem).expect("build credential");
        assert_eq!(
            credential.display_name(),
            AgentAccount::LocalSystem.display_name()
        );
        assert!(credential.reuses_supervisor_token());
    }
}
