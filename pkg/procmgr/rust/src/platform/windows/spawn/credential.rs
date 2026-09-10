// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use windows_sys::Win32::Foundation::HANDLE;

use super::super::local_agent_account::AgentAccount;
#[cfg(not(test))]
use super::super::local_agent_account::resolve_agent_account;
use super::logon::{logon_user_credentials, logon_user_token};
use super::win32::duplicate_primary_token;

/// Resolved spawn identity for Windows spawn.
///
/// If you are not familiar with Windows: we either reuse procmgrd's access token (when it
/// already runs as the target account) or call `LogonUser` for a primary token. Password
/// retrieval is a follow-up (A4-lsa). This type only holds the resolved account and which
/// path to take.
#[derive(Clone, Debug)]
pub(crate) struct SpawnCredential {
    account: AgentAccount,
    reuses_supervisor_token: bool,
}

impl SpawnCredential {
    /// Privileged children inherit dd-procmgrd's token via `CreateProcessW`.
    ///
    /// Always `reuses_supervisor_token`: we do not `LogonUser` as LocalSystem, and we do
    /// not use `CreateProcessAsUserW` (that API needs `SeIncreaseQuotaPrivilege`, which
    /// the installed agent account does not hold).
    pub(crate) fn privileged() -> Self {
        Self {
            account: AgentAccount::LocalSystem,
            reuses_supervisor_token: true,
        }
    }

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

    /// Primary token for `CreateProcessAsUserW` (required Win32 token type for new processes).
    ///
    /// LogonUser path only. Same-account spawns use `CreateProcessW` and do not call this.
    pub(crate) fn duplicate_primary_token(&self, process_name: &str) -> Result<HANDLE> {
        duplicate_primary_token(
            process_name,
            logon_user_token(process_name, &logon_user_credentials(&self.account))?.raw(),
        )
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
    fn privileged_spawn_always_inherits_supervisor_token() {
        let credential = SpawnCredential::privileged();
        assert!(credential.reuses_supervisor_token());
        assert_eq!(
            credential.display_name(),
            AgentAccount::LocalSystem.display_name()
        );
    }

    #[test]
    fn logon_credential_display_name_matches_account() {
        let credential =
            SpawnCredential::from_account(AgentAccount::LocalSystem).expect("build credential");
        assert_eq!(
            credential.display_name(),
            AgentAccount::LocalSystem.display_name()
        );
        assert_eq!(
            credential.reuses_supervisor_token(),
            AgentAccount::LocalSystem
                .reuses_supervisor_token()
                .expect("compare LocalSystem to supervisor token")
        );
    }
}
