// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows spawn credentials: how CreateProcessAsUserW obtains its primary token.

use anyhow::{Result, anyhow, bail};
use windows_sys::Win32::Foundation::HANDLE;
use windows_sys::Win32::Security::{TOKEN_DUPLICATE, TOKEN_QUERY};

use crate::spawn::SpawnProfile;

use super::super::agent_credentials::AgentAccount;
#[cfg(not(test))]
use super::super::agent_credentials::resolve_agent_account;
use super::super::token_identity::{open_current_process_token, token_user_is_local_system};
use super::logon::{logon_user_credentials, logon_user_token};
use super::win32::duplicate_primary_token;

const SYSTEM_DISPLAY: &str = "NT AUTHORITY\\SYSTEM";

/// Primary-token strategy for a spawn.
///
/// Every spawn uses one of two paths:
/// - [`Self::InheritSupervisor`]: duplicate dd-procmgrd's token (LocalSystem service or test harness)
/// - [`Self::Logon`]: LogonUserW for a distinct installer account (ddagentuser / gMSA)
#[derive(Debug)]
pub(crate) enum SpawnCredential {
    InheritSupervisor {
        display_name: String,
        /// Privileged spawn and production LocalSystem agent spawn require this.
        require_local_system: bool,
    },
    #[cfg_attr(test, allow(dead_code))]
    Logon(AgentAccount),
}

impl SpawnCredential {
    pub(crate) fn resolve(profile: SpawnProfile) -> Result<Self> {
        match profile {
            SpawnProfile::Privileged => Ok(Self::privileged_local_system()),
            SpawnProfile::Agent => Self::resolve_agent_profile(),
        }
    }

    fn privileged_local_system() -> Self {
        Self::InheritSupervisor {
            display_name: SYSTEM_DISPLAY.to_string(),
            require_local_system: true,
        }
    }

    fn resolve_agent_profile() -> Result<Self> {
        #[cfg(test)]
        {
            super::test_harness::agent_profile_credential()
        }
        #[cfg(not(test))]
        {
            match resolve_agent_account()? {
                AgentAccount::LocalSystem => Ok(Self::privileged_local_system()),
                account => Ok(Self::Logon(account)),
            }
        }
    }

    pub(crate) fn display_name(&self) -> String {
        match self {
            Self::InheritSupervisor { display_name, .. } => display_name.clone(),
            Self::Logon(account) => account.display_name(),
        }
    }

    pub(crate) fn inherits_supervisor_token(&self) -> bool {
        matches!(self, Self::InheritSupervisor { .. })
    }

    pub(crate) fn agent_account_for_interactive_logon(&self) -> Option<&AgentAccount> {
        match self {
            Self::Logon(account) => Some(account),
            Self::InheritSupervisor { .. } => None,
        }
    }

    pub(crate) fn duplicate_primary_token(&self, process_name: &str) -> Result<HANDLE> {
        match self {
            Self::InheritSupervisor {
                require_local_system,
                ..
            } => inherit_supervisor_primary_token(process_name, *require_local_system),
            Self::Logon(account) => duplicate_primary_token(
                process_name,
                logon_user_token(process_name, &logon_user_credentials(account))?.raw(),
            ),
        }
    }
}

fn inherit_supervisor_primary_token(
    process_name: &str,
    require_local_system: bool,
) -> Result<HANDLE> {
    let process_token = open_current_process_token(TOKEN_QUERY | TOKEN_DUPLICATE).map_err(|e| {
        anyhow!("[{process_name}] OpenProcessToken(GetCurrentProcess()) failed: {e}")
    })?;
    if require_local_system
        && !token_user_is_local_system(process_token.as_handle())
            .map_err(|e| anyhow!("[{process_name}] verify supervisor token is LocalSystem: {e}"))?
    {
        bail!("[{process_name}] privileged spawn requires dd-procmgrd to run as LocalSystem");
    }
    duplicate_primary_token(process_name, process_token.as_handle())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn privileged_profile_inherits_local_system_supervisor() {
        let credential =
            SpawnCredential::resolve(SpawnProfile::Privileged).expect("resolve privileged");
        assert!(matches!(
            credential,
            SpawnCredential::InheritSupervisor {
                require_local_system: true,
                ..
            }
        ));
        assert_eq!(credential.display_name(), SYSTEM_DISPLAY);
    }

    #[test]
    fn agent_profile_in_unit_tests_never_logons_installer_account() {
        let credential =
            SpawnCredential::resolve(SpawnProfile::Agent).expect("resolve agent profile in tests");
        assert!(
            matches!(credential, SpawnCredential::InheritSupervisor { .. }),
            "unit tests must not resolve registry credentials: {credential:?}"
        );
    }
}
