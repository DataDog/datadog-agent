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
use super::logon::{logon_user_credentials, logon_user_token};
use super::win32::duplicate_primary_token;

/// Resolved spawn identity for agent-profile LogonUser / CreateProcessAsUserW spawn.
///
/// Privileged spawn credentials are deferred to A5 (privileged profile still uses
/// `Command::spawn` inherit until then).
#[derive(Debug)]
pub(crate) struct SpawnCredential(AgentAccount);

impl SpawnCredential {
    pub(crate) fn resolve_agent() -> Result<Self> {
        #[cfg(test)]
        {
            super::test_harness::agent_profile_credential()
        }
        #[cfg(not(test))]
        {
            Ok(Self(resolve_agent_account()?))
        }
    }

    pub(crate) fn display_name(&self) -> String {
        self.account().display_name()
    }

    pub(crate) fn account(&self) -> &AgentAccount {
        &self.0
    }

    pub(crate) fn duplicate_primary_token(&self, process_name: &str) -> Result<HANDLE> {
        if self
            .account()
            .spawns_with_supervisor_token()
            .with_context(|| {
                format!("[{process_name}] compare spawn account to supervisor token")
            })?
        {
            let supervisor_token = open_current_process_token(TOKEN_QUERY | TOKEN_DUPLICATE)
                .map_err(|e| {
                    anyhow::anyhow!(
                        "[{process_name}] OpenProcessToken(GetCurrentProcess()) failed: {e}"
                    )
                })?;
            return duplicate_primary_token(process_name, supervisor_token.as_handle());
        }

        let account = self.account();
        duplicate_primary_token(
            process_name,
            logon_user_token(process_name, &logon_user_credentials(account))?.raw(),
        )
    }
}

#[cfg(test)]
impl SpawnCredential {
    pub(super) fn from_account(account: AgentAccount) -> Self {
        Self(account)
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
            | AgentAccount::NetworkService => {}
            AgentAccount::PasswordLogon { password, .. } => {
                assert!(
                    password.is_empty(),
                    "test harness must inherit the supervisor token, not SCM installer password: \
                     {credential:?}"
                );
                assert!(
                    credential
                        .account()
                        .spawns_with_supervisor_token()
                        .expect("compare spawn account to supervisor token"),
                    "empty-password PasswordLogon must inherit the supervisor token: {credential:?}"
                );
            }
        }
    }

    #[test]
    fn logon_credential_display_name_matches_account() {
        let credential = SpawnCredential::from_account(AgentAccount::LocalSystem);
        assert_eq!(
            credential.display_name(),
            AgentAccount::LocalSystem.display_name()
        );
    }
}
