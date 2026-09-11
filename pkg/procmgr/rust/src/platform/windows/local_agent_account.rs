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

use super::agent_service_sid::lookup_installed_user_sid;
#[cfg(not(test))]
use super::local_account::is_local_account;
use super::sid::create_well_known_sid;
use super::token_identity::current_process_sid_matches;
#[cfg(not(test))]
use super::{open_datadog_agent_key, registry_nonempty_string};

const NT_AUTHORITY: &str = "NT AUTHORITY";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AccountName {
    domain: String,
    user: String,
}

impl AccountName {
    pub(crate) fn new(domain: impl Into<String>, user: impl Into<String>) -> Self {
        Self {
            domain: domain.into(),
            user: user.into(),
        }
    }

    pub(crate) fn display(&self) -> String {
        if self.domain.is_empty() {
            format!(r".\{}", self.user)
        } else {
            format!("{}\\{}", self.domain, self.user)
        }
    }
}

#[derive(Clone, PartialEq, Eq)]
pub(crate) enum AgentAccount {
    LocalSystem,
    LocalService,
    NetworkService,
    SupervisorAccount {
        registry_domain: String,
        logon_domain: String,
        user: String,
    },
    #[allow(dead_code)]
    PasswordLogon {
        registry_domain: String,
        logon_domain: String,
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
            Self::SupervisorAccount {
                registry_domain,
                logon_domain,
                user,
            } => f
                .debug_struct("SupervisorAccount")
                .field("registry_domain", registry_domain)
                .field("logon_domain", logon_domain)
                .field("user", user)
                .finish(),
            Self::PasswordLogon {
                registry_domain,
                logon_domain,
                user,
                ..
            } => f
                .debug_struct("PasswordLogon")
                .field("registry_domain", registry_domain)
                .field("logon_domain", logon_domain)
                .field("user", user)
                .field("password", &"****")
                .finish(),
        }
    }
}

impl AgentAccount {
    pub(crate) fn reuses_supervisor_token(&self) -> Result<bool> {
        match self {
            AgentAccount::SupervisorAccount { .. } => Ok(true),
            AgentAccount::LocalSystem
            | AgentAccount::LocalService
            | AgentAccount::NetworkService => {
                let sid = self
                    .well_known_account_sid()
                    .expect("well-known service account must have a SID")
                    .with_context(|| format!("lookup SID for {}", self.display_name()))?;
                current_process_sid_matches(&sid)
                    .with_context(|| format!("compare supervisor token to {}", self.display_name()))
            }
            AgentAccount::PasswordLogon {
                logon_domain, user, ..
            } => {
                let sid = lookup_installed_user_sid(logon_domain, user)
                    .with_context(|| format!("lookup SID for {}", self.display_name()))?;
                current_process_sid_matches(&sid)
                    .with_context(|| format!("compare supervisor token to {}", self.display_name()))
            }
        }
    }

    pub(crate) fn display_name(&self) -> String {
        match self {
            AgentAccount::SupervisorAccount {
                registry_domain,
                user,
                ..
            }
            | AgentAccount::PasswordLogon {
                registry_domain,
                user,
                ..
            } => AccountName::new(registry_domain, user).display(),
            _ => self.logon_account_name().display(),
        }
    }

    pub(crate) fn logon_account_name(&self) -> AccountName {
        match self {
            AgentAccount::LocalSystem => AccountName::new(NT_AUTHORITY, "SYSTEM"),
            AgentAccount::LocalService => AccountName::new(NT_AUTHORITY, "LocalService"),
            AgentAccount::NetworkService => AccountName::new(NT_AUTHORITY, "NetworkService"),
            AgentAccount::SupervisorAccount {
                logon_domain, user, ..
            }
            | AgentAccount::PasswordLogon {
                logon_domain, user, ..
            } => AccountName::new(logon_domain, user),
        }
    }

    fn well_known_account_sid(&self) -> Option<Result<Vec<u8>>> {
        let well_known = match self {
            AgentAccount::LocalSystem => WinLocalSystemSid,
            AgentAccount::LocalService => WinLocalServiceSid,
            AgentAccount::NetworkService => WinNetworkServiceSid,
            _ => return None,
        };
        Some(create_well_known_sid(well_known))
    }
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
fn stored_logon_domain(registry_domain: &str, sid: &[u8]) -> Result<String> {
    Ok(normalize_registry_domain_for_logon(
        registry_domain,
        is_local_account(sid)?,
    ))
}

fn normalize_registry_domain_for_logon(registry_domain: &str, is_local: bool) -> String {
    if is_local {
        String::new()
    } else {
        registry_domain.to_string()
    }
}

#[cfg(not(test))]
fn resolve_local_agent_account(domain: String, user: String, sid: &[u8]) -> Result<AgentAccount> {
    let display = AccountName::new(&domain, &user).display();

    if current_process_sid_matches(sid)
        .with_context(|| format!("compare supervisor token to installed agent account {display}"))?
    {
        info!(
            "dd-procmgrd runs as installed agent account {display}; inheriting supervisor token for agent spawn"
        );
        let logon_domain = stored_logon_domain(&domain, sid)?;
        return Ok(AgentAccount::SupervisorAccount {
            registry_domain: domain,
            logon_domain,
            user,
        });
    }

    let is_local =
        is_local_account(sid).with_context(|| format!("classify local account for {display}"))?;
    if !is_local {
        bail!("domain agent account {display} is not supported");
    }

    bail!(
        "agent user password is not available for local account {display}; \
         run dd-procmgrd as the installed agent account"
    );
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
    use super::*;

    fn local_system() -> AccountName {
        AccountName::new(NT_AUTHORITY, "SYSTEM")
    }

    fn local_service() -> AccountName {
        AccountName::new(NT_AUTHORITY, "LocalService")
    }

    fn network_service() -> AccountName {
        AccountName::new(NT_AUTHORITY, "NetworkService")
    }

    #[test]
    fn account_name_display_formats_well_known_accounts() {
        assert_eq!(local_system().display(), r"NT AUTHORITY\SYSTEM");
        assert_eq!(local_service().display(), r"NT AUTHORITY\LocalService");
        assert_eq!(network_service().display(), r"NT AUTHORITY\NetworkService");
    }

    #[test]
    fn account_name_display_formats_local_and_domain_accounts() {
        assert_eq!(
            AccountName::new("", "ddagentuser").display(),
            r".\ddagentuser"
        );
        assert_eq!(AccountName::new("CORP", "gmsa$").display(), r"CORP\gmsa$");
    }

    #[test]
    fn display_name_formats_accounts() {
        assert_eq!(
            AgentAccount::LocalSystem.display_name(),
            AccountName::new(NT_AUTHORITY, "SYSTEM").display(),
        );
        assert_eq!(
            AgentAccount::SupervisorAccount {
                registry_domain: "WIN-HOST".to_string(),
                logon_domain: String::new(),
                user: "ddagentuser".to_string(),
            }
            .display_name(),
            r"WIN-HOST\ddagentuser",
        );
        assert_eq!(
            AgentAccount::PasswordLogon {
                registry_domain: "WIN-HOST".to_string(),
                logon_domain: String::new(),
                user: "ddagentuser".to_string(),
                password: "secret".to_string(),
            }
            .display_name(),
            r"WIN-HOST\ddagentuser",
        );
    }

    #[test]
    fn supervisor_registry_display_differs_from_logon_account_name() {
        let account = AgentAccount::SupervisorAccount {
            registry_domain: "WIN-HOST".to_string(),
            logon_domain: String::new(),
            user: "ddagentuser".to_string(),
        };
        assert_eq!(account.display_name(), r"WIN-HOST\ddagentuser");
        assert_eq!(account.logon_account_name().display(), r".\ddagentuser");
    }

    #[test]
    fn supervisor_account_always_reuses_supervisor_token() {
        let account = AgentAccount::SupervisorAccount {
            registry_domain: String::new(),
            logon_domain: String::new(),
            user: "ddagentuser".to_string(),
        };
        assert!(account.reuses_supervisor_token().unwrap());
    }

    #[test]
    fn stored_logon_domain_clears_stale_hostname_for_local_accounts() {
        assert_eq!(
            normalize_registry_domain_for_logon("OLD-HOST", true),
            String::new()
        );
        assert_eq!(
            normalize_registry_domain_for_logon("CORP", false),
            "CORP".to_string()
        );
    }

    #[test]
    fn well_known_account_sid_resolves_service_accounts() {
        let local_service = AgentAccount::LocalService
            .well_known_account_sid()
            .expect("LocalService SID")
            .expect("lookup LocalService SID");
        assert!(is_local_service_sid(&local_service));

        let network_service = AgentAccount::NetworkService
            .well_known_account_sid()
            .expect("NetworkService SID")
            .expect("lookup NetworkService SID");
        assert!(is_network_service_sid(&network_service));

        let local_system = AgentAccount::LocalSystem
            .well_known_account_sid()
            .expect("LocalSystem SID")
            .expect("lookup LocalSystem SID");
        assert!(is_local_system_sid(&local_system));
    }
}
