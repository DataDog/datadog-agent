// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(not(test))]
use anyhow::bail;
use anyhow::{Context, Result};
#[cfg(not(test))]
use log::info;
use windows_sys::Win32::Security::{
    IsWellKnownSid, WinLocalServiceSid, WinLocalSystemSid, WinNetworkServiceSid,
};

use super::account_name::AccountName;
#[cfg(not(test))]
use super::agent_service_sid::lookup_installed_user_sid;
#[cfg(not(test))]
use super::agent_service_sid::{DATADOG_AGENT_SERVICE, service_runs_as_agent_user};
use super::local_account::is_local_account;
#[cfg(not(test))]
use super::scm_lsa_secret::read_scm_service_password;
use super::sid::lookup_account_sid;
use super::token_identity::current_process_sid_matches;
#[cfg(not(test))]
use super::{open_datadog_agent_key, registry_nonempty_string};

const NT_AUTHORITY: &str = "NT AUTHORITY";

#[derive(Clone, PartialEq, Eq)]
pub(crate) enum AgentAccount {
    LocalSystem,
    LocalService,
    NetworkService,
    PasswordLogon {
        domain: String,
        user: String,
        password: String,
    },
}

impl std::fmt::Debug for AgentAccount {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::LocalSystem => f.write_str("LocalSystem"),
            Self::LocalService => f.write_str("LocalService"),
            Self::NetworkService => f.write_str("NetworkService"),
            Self::PasswordLogon { domain, user, .. } => f
                .debug_struct("PasswordLogon")
                .field("domain", domain)
                .field("user", user)
                .field("password", &"****")
                .finish(),
        }
    }
}

impl AgentAccount {
    pub(crate) fn inherits_supervisor_token(&self) -> bool {
        matches!(self, AgentAccount::LocalSystem)
    }

    pub(crate) fn spawns_with_supervisor_token(&self) -> Result<bool> {
        if self.inherits_supervisor_token() {
            return Ok(true);
        }
        let AgentAccount::PasswordLogon { domain, user, .. } = self else {
            return Ok(false);
        };
        let sid = lookup_account_sid(domain, user)
            .with_context(|| format!("lookup SID for {}", self.display_name()))?;
        current_process_sid_matches(&sid)
            .with_context(|| format!("compare supervisor token to {}", self.display_name()))
    }

    pub(crate) fn display_name(&self) -> String {
        self.account_name().display()
    }

    pub(crate) fn account_name(&self) -> AccountName {
        match self {
            AgentAccount::LocalSystem => AccountName::new(NT_AUTHORITY, "SYSTEM"),
            AgentAccount::LocalService => AccountName::new(NT_AUTHORITY, "LocalService"),
            AgentAccount::NetworkService => AccountName::new(NT_AUTHORITY, "NetworkService"),
            AgentAccount::PasswordLogon { domain, user, .. } => {
                account_name_for_logon(domain, user)
            }
        }
    }
}

fn account_name_for_logon(domain: &str, user: &str) -> AccountName {
    let display_domain = match lookup_account_sid(domain, user)
        .ok()
        .and_then(|sid| is_local_account(&sid).ok())
    {
        Some(true) => String::new(),
        _ => domain.to_string(),
    };
    AccountName::new(display_domain, user)
}

#[cfg(not(test))]
pub(crate) fn resolve_agent_account() -> Result<AgentAccount> {
    let Some(key) = open_datadog_agent_key() else {
        bail!("open HKLM\\SOFTWARE\\Datadog\\Datadog Agent");
    };
    let user = registry_nonempty_string(&key, "installedUser")
        .context("read installedUser from registry")?;
    let domain = key
        .get_string("installedDomain")
        .unwrap_or_default()
        .trim()
        .to_string();

    if let Some(account) = well_known_from_names(&domain, &user) {
        return Ok(account);
    }

    let sid = lookup_installed_user_sid(&domain, &user)
        .with_context(|| format!("lookup SID for {domain}\\{user}"))?;
    if let Some(account) = well_known_from_sid(&sid) {
        return Ok(account);
    }

    resolve_local_agent_account(domain, user, &sid)
}

#[cfg(not(test))]
fn resolve_local_agent_account(domain: String, user: String, sid: &[u8]) -> Result<AgentAccount> {
    let display = AccountName::new(&domain, &user).display();

    if current_process_sid_matches(sid)
        .with_context(|| format!("compare supervisor token to installed agent account {display}"))?
    {
        info!(
            "dd-procmgrd runs as installed agent account {display}; \
             agent spawn will inherit the supervisor token until LocalSystem migration"
        );
        return Ok(AgentAccount::PasswordLogon {
            domain,
            user,
            password: String::new(),
        });
    }

    let is_local =
        is_local_account(sid).with_context(|| format!("classify local account for {display}"))?;
    if !is_local {
        bail!("domain agent account {display} is not supported");
    }

    let scm_service_matches_agent =
        service_runs_as_agent_user(DATADOG_AGENT_SERVICE, &domain, &user)?;
    let scm_password = if scm_service_matches_agent {
        read_scm_service_password(DATADOG_AGENT_SERVICE)?
    } else {
        None
    };
    info!(
        "local agent account inputs for {display}: service_account_matches={scm_service_matches_agent}, password_present={}",
        scm_password
            .as_ref()
            .is_some_and(|password| !password.is_empty())
    );

    let password = scm_password
        .filter(|password| !password.is_empty())
        .with_context(|| {
            format!(
                "agent user password is not available for local account {display}; \
                 ensure datadogagent service runs as the installed agent user"
            )
        })?;

    Ok(AgentAccount::PasswordLogon {
        domain,
        user,
        password,
    })
}

#[cfg(not(test))]
fn well_known_from_names(domain: &str, user: &str) -> Option<AgentAccount> {
    if is_local_system_name(domain, user) {
        Some(AgentAccount::LocalSystem)
    } else if is_local_service_name(domain, user) {
        Some(AgentAccount::LocalService)
    } else if is_network_service_name(domain, user) {
        Some(AgentAccount::NetworkService)
    } else {
        None
    }
}

#[cfg(test)]
pub(crate) fn agent_account_from_well_known_sid(sid: &[u8]) -> Option<AgentAccount> {
    well_known_from_sid(sid)
}

fn well_known_from_sid(sid: &[u8]) -> Option<AgentAccount> {
    if is_local_system_sid(sid) {
        Some(AgentAccount::LocalSystem)
    } else if is_local_service_sid(sid) {
        Some(AgentAccount::LocalService)
    } else if is_network_service_sid(sid) {
        Some(AgentAccount::NetworkService)
    } else {
        None
    }
}

#[cfg(not(test))]
fn is_local_system_name(domain: &str, user: &str) -> bool {
    (domain.is_empty() && user.eq_ignore_ascii_case("LocalSystem"))
        || (domain.eq_ignore_ascii_case("NT AUTHORITY") && user.eq_ignore_ascii_case("SYSTEM"))
}

#[cfg(not(test))]
fn is_local_service_name(domain: &str, user: &str) -> bool {
    (domain.is_empty() && user.eq_ignore_ascii_case("LocalService"))
        || (domain.eq_ignore_ascii_case("NT AUTHORITY")
            && user.eq_ignore_ascii_case("LOCAL SERVICE"))
}

#[cfg(not(test))]
fn is_network_service_name(domain: &str, user: &str) -> bool {
    (domain.is_empty() && user.eq_ignore_ascii_case("NetworkService"))
        || (domain.eq_ignore_ascii_case("NT AUTHORITY")
            && user.eq_ignore_ascii_case("NETWORK SERVICE"))
}

fn is_local_system_sid(sid: &[u8]) -> bool {
    is_well_known_sid(sid, WinLocalSystemSid)
}

fn is_local_service_sid(sid: &[u8]) -> bool {
    is_well_known_sid(sid, WinLocalServiceSid)
}

fn is_network_service_sid(sid: &[u8]) -> bool {
    is_well_known_sid(sid, WinNetworkServiceSid)
}

fn is_well_known_sid(
    sid: &[u8],
    well_known: windows_sys::Win32::Security::WELL_KNOWN_SID_TYPE,
) -> bool {
    unsafe { IsWellKnownSid(sid.as_ptr() as *mut _, well_known) != 0 }
}

#[cfg(test)]
mod tests {
    use super::super::account_name::AccountName;
    use super::*;

    #[test]
    fn display_name_formats_accounts() {
        assert_eq!(
            AgentAccount::LocalSystem.display_name(),
            AccountName::new(NT_AUTHORITY, "SYSTEM").display(),
        );
        assert_eq!(
            AgentAccount::PasswordLogon {
                domain: String::new(),
                user: "ddagentuser".to_string(),
                password: "secret".to_string(),
            }
            .display_name(),
            AccountName::new("", "ddagentuser").display(),
        );
    }
}
