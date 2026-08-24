// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, bail};
#[cfg(not(test))]
use log::info;
#[cfg(not(test))]
use std::ptr;
#[cfg(not(test))]
use windows_sys::Win32::Security::Authentication::Identity::{
    LSA_HANDLE, LSA_OBJECT_ATTRIBUTES, LSA_UNICODE_STRING, LsaClose, LsaFreeMemory, LsaOpenPolicy,
    LsaRetrievePrivateData, POLICY_GET_PRIVATE_INFORMATION,
};
use windows_sys::Win32::Security::{
    IsWellKnownSid, WinLocalServiceSid, WinLocalSystemSid, WinNetworkServiceSid,
};

use super::account_name::AccountName;
#[cfg(not(test))]
use super::agent_service_sid::{
    DATADOG_AGENT_SERVICE, lookup_installed_user_sid, service_runs_as_agent_user,
};
use super::local_account::is_local_account;
use super::managed_service_account::ManagedServiceAccountState;
#[cfg(not(test))]
use super::managed_service_account::query_managed_service_account;
use super::sid::lookup_account_sid;
#[cfg(not(test))]
use super::wide;
#[cfg(not(test))]
use super::{open_datadog_agent_key, registry_nonempty_string};

#[cfg(not(test))]
const AGENT_PASSWORD_LSA_KEY: &str = "L$datadog_ddagentuser_password";
#[cfg(not(test))]
const SCM_SERVICE_PASSWORD_LSA_PREFIX: &str = "_SC_";
#[cfg(not(test))]
const STATUS_OBJECT_NAME_NOT_FOUND: i32 = 0xC000_0034u32 as i32;
const NT_AUTHORITY: &str = "NT AUTHORITY";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum AgentAccount {
    LocalSystem,
    LocalService,
    NetworkService,
    PasswordLogon {
        domain: String,
        user: String,
        password: String,
    },
    ServiceAccountLogon {
        domain: String,
        user: String,
    },
}

impl AgentAccount {
    pub(crate) fn inherits_supervisor_token(&self) -> bool {
        matches!(self, AgentAccount::LocalSystem)
    }

    /// Operator-facing account name for list/describe output.
    pub(crate) fn display_name(&self) -> String {
        self.account_name().display()
    }

    fn account_name(&self) -> AccountName {
        match self {
            AgentAccount::LocalSystem => AccountName::new(NT_AUTHORITY, "SYSTEM"),
            AgentAccount::LocalService => AccountName::new(NT_AUTHORITY, "LocalService"),
            AgentAccount::NetworkService => AccountName::new(NT_AUTHORITY, "NetworkService"),
            AgentAccount::PasswordLogon { domain, user, .. }
            | AgentAccount::ServiceAccountLogon { domain, user } => {
                account_name_for_logon(domain, user)
            }
        }
    }
}

/// Match registry-style local SAM display (`.\user`) when installer stored the computer name as domain.
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

/// Resolve the spawn account display string for a profile on Windows.
#[cfg(any(test, feature = "test-helpers"))]
pub(crate) fn spawn_user_for_profile(
    process_name: &str,
    profile: crate::spawn::SpawnProfile,
) -> Result<String> {
    use super::spawn::SpawnCredential;

    SpawnCredential::resolve(profile)
        .with_context(|| format!("[{process_name}] resolve spawn credential for spawn user"))
        .map(|credential| credential.display_name())
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

    resolve_domain_agent_account(domain, user, &sid)
}

#[cfg(not(test))]
fn resolve_domain_agent_account(domain: String, user: String, sid: &[u8]) -> Result<AgentAccount> {
    let display = AccountName::new(&domain, &user).display();
    info!("resolving domain agent account for {display}");
    let is_local =
        is_local_account(sid).with_context(|| format!("classify local account for {display}"))?;
    let lsa_password = read_agent_password(&domain, &user)?;
    let msa = if should_query_managed_service_account(&user, lsa_password.as_deref()) {
        query_managed_service_account(&domain, &user)?
    } else {
        ManagedServiceAccountState::AssumeRegularDomainAccount
    };
    info!(
        "domain agent account inputs for {display}: is_local={is_local}, lsa_password_present={}, msa={msa:?}",
        lsa_password
            .as_ref()
            .is_some_and(|password| !password.is_empty())
    );
    agent_account_from_msa_and_lsa(domain, user, lsa_password.as_deref(), is_local, msa)
}

/// Whether NetQueryServiceAccount is needed to pick the spawn credential type.
///
/// Skip the DC-dependent query when a regular domain account already has a stored password.
fn should_query_managed_service_account(user: &str, lsa_password: Option<&str>) -> bool {
    user.ends_with('$')
        || lsa_password
            .filter(|password| !password.is_empty())
            .is_none()
}

/// Pick the spawn credential type from gMSA classification and optional LSA password.
///
/// Installed gMSA wins over a stale LSA secret left behind when installer secret removal fails
/// during a password-account to gMSA migration. When gMSA classification is unavailable, regular
/// domain accounts fall back to the stored password instead of failing the DC query.
fn agent_account_from_msa_and_lsa(
    domain: String,
    user: String,
    lsa_password: Option<&str>,
    is_local: bool,
    msa: ManagedServiceAccountState,
) -> Result<AgentAccount> {
    if matches!(msa, ManagedServiceAccountState::Installed) {
        return Ok(AgentAccount::ServiceAccountLogon { domain, user });
    }
    if let Some(password) = lsa_password.filter(|password| !password.is_empty())
        && should_use_lsa_password(&user, msa)
    {
        return Ok(AgentAccount::PasswordLogon {
            domain,
            user,
            password: password.to_string(),
        });
    }
    info!(
        "no usable LSA password for {}; attempting passwordless resolution with msa={msa:?}",
        AccountName::new(&domain, &user).display()
    );
    passwordless_agent_account(domain, user, is_local, msa)
}

fn should_use_lsa_password(user: &str, msa: ManagedServiceAccountState) -> bool {
    match msa {
        ManagedServiceAccountState::NotService
        | ManagedServiceAccountState::AssumeRegularDomainAccount => true,
        ManagedServiceAccountState::ClassificationUnavailable => !user.ends_with('$'),
        ManagedServiceAccountState::Installed
        | ManagedServiceAccountState::NotExist
        | ManagedServiceAccountState::CannotInstall
        | ManagedServiceAccountState::CanInstall => false,
    }
}

fn passwordless_agent_account(
    domain: String,
    user: String,
    is_local: bool,
    msa: ManagedServiceAccountState,
) -> Result<AgentAccount> {
    let display = AccountName::new(&domain, &user).display();
    if is_local {
        bail!(
            "agent user password is not available for local account {display}; \
             reinstall the Agent with the password provided"
        );
    }

    match msa {
        ManagedServiceAccountState::Installed => {
            Ok(AgentAccount::ServiceAccountLogon { domain, user })
        }
        ManagedServiceAccountState::ClassificationUnavailable if user.ends_with('$') => {
            Ok(AgentAccount::ServiceAccountLogon { domain, user })
        }
        ManagedServiceAccountState::NotService
        | ManagedServiceAccountState::AssumeRegularDomainAccount
        | ManagedServiceAccountState::ClassificationUnavailable => {
            if user.ends_with('$') {
                bail!(
                    "account {display} ends with '$' but is not recognized as a valid gMSA account; \
                     reinstall the Agent with the password provided if this is a normal account"
                );
            }
            bail!(
                "agent user password is not available for {display}; \
                 reinstall the Agent with the password provided"
            );
        }
        ManagedServiceAccountState::NotExist => bail!("account {display} does not exist"),
        ManagedServiceAccountState::CannotInstall => {
            bail!("account {display} is a gMSA account but cannot be installed on this host")
        }
        ManagedServiceAccountState::CanInstall => {
            bail!("unexpected gMSA install state for account {display}")
        }
    }
}

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

/// Canonical operator-facing name for built-in service SIDs.
///
/// `LookupAccountSidW` spells LocalService and NetworkService with spaces; installer
/// state and spawn display use the compact forms instead.
pub(crate) fn canonical_account_name_for_well_known_sid(sid: &[u8]) -> Option<AccountName> {
    well_known_from_sid(sid).map(|account| account.account_name())
}

fn is_local_system_name(domain: &str, user: &str) -> bool {
    (domain.is_empty() && user.eq_ignore_ascii_case("LocalSystem"))
        || (domain.eq_ignore_ascii_case("NT AUTHORITY") && user.eq_ignore_ascii_case("SYSTEM"))
}

fn is_local_service_name(domain: &str, user: &str) -> bool {
    (domain.is_empty() && user.eq_ignore_ascii_case("LocalService"))
        || (domain.eq_ignore_ascii_case("NT AUTHORITY")
            && user.eq_ignore_ascii_case("LOCAL SERVICE"))
}

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

/// Pick the first non-empty Agent password from installer LSA storage or SCM service credentials.
fn agent_password_from_sources(
    installer_lsa_password: Option<&str>,
    scm_service_password: Option<&str>,
    scm_service_matches_agent: bool,
) -> Option<String> {
    if let Some(password) = installer_lsa_password.filter(|password| !password.is_empty()) {
        return Some(password.to_string());
    }
    if scm_service_matches_agent {
        scm_service_password
            .filter(|password| !password.is_empty())
            .map(str::to_string)
    } else {
        None
    }
}

#[cfg(not(test))]
fn read_agent_password(domain: &str, user: &str) -> Result<Option<String>> {
    let installer_lsa_password = read_lsa_private_data(AGENT_PASSWORD_LSA_KEY)?;
    if installer_lsa_password
        .as_ref()
        .is_some_and(|password| !password.is_empty())
    {
        info!("using installer LSA password for {domain}\\{user}");
        return Ok(installer_lsa_password);
    }

    let scm_service_matches_agent =
        service_runs_as_agent_user(DATADOG_AGENT_SERVICE, domain, user)?;
    let scm_password = if scm_service_matches_agent {
        let scm_key = format!("{SCM_SERVICE_PASSWORD_LSA_PREFIX}{DATADOG_AGENT_SERVICE}");
        read_lsa_private_data(&scm_key)?
    } else {
        None
    };
    info!(
        "agent password fallback for {domain}\\{user}: installer_lsa_present=false, scm_service_matches_agent={scm_service_matches_agent}, scm_password_present={}",
        scm_password
            .as_ref()
            .is_some_and(|password| !password.is_empty())
    );
    Ok(agent_password_from_sources(
        None,
        scm_password.as_deref(),
        scm_service_matches_agent,
    ))
}

#[cfg(not(test))]
fn read_lsa_private_data(key: &str) -> Result<Option<String>> {
    let mut key_w = wide::null_terminated(key);
    let key_len = key_w.len().saturating_sub(1);
    let key_name = LSA_UNICODE_STRING {
        Length: (key_len * 2) as u16,
        MaximumLength: (key_w.len() * 2) as u16,
        Buffer: key_w.as_mut_ptr(),
    };

    unsafe {
        let object_attributes: LSA_OBJECT_ATTRIBUTES = std::mem::zeroed();
        let mut policy_handle: LSA_HANDLE = 0;

        let status = LsaOpenPolicy(
            ptr::null(),
            &object_attributes,
            POLICY_GET_PRIVATE_INFORMATION as u32,
            &mut policy_handle,
        );
        if status != 0 {
            bail!("LsaOpenPolicy: NTSTATUS {status:#010x}");
        }

        let policy = PolicyHandle(policy_handle);
        let mut secret: *mut LSA_UNICODE_STRING = ptr::null_mut();
        let status = LsaRetrievePrivateData(policy.0, &key_name, &mut secret);

        if status == STATUS_OBJECT_NAME_NOT_FOUND {
            return Ok(None);
        }
        if status != 0 {
            bail!("LsaRetrievePrivateData: NTSTATUS {status:#010x}");
        }
        if secret.is_null() {
            return Ok(None);
        }

        let secret_ref = &*secret;
        let char_count = secret_ref.Length as usize / 2;
        let password = if char_count == 0 {
            String::new()
        } else {
            let slice = std::slice::from_raw_parts(secret_ref.Buffer, char_count);
            String::from_utf16_lossy(slice)
        };

        LsaFreeMemory(secret as _);
        Ok(Some(password))
    }
}

#[cfg(not(test))]
struct PolicyHandle(LSA_HANDLE);

#[cfg(not(test))]
impl Drop for PolicyHandle {
    fn drop(&mut self) {
        if self.0 != 0 {
            unsafe {
                LsaClose(self.0);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::account_name::AccountName;
    use super::*;

    #[test]
    fn local_system_name_detection() {
        assert!(is_local_system_name("", "LocalSystem"));
        assert!(is_local_system_name("NT AUTHORITY", "SYSTEM"));
        assert!(!is_local_system_name("WIN-HOST", "ddagentuser"));
        assert!(!is_local_system_name("CORP", "LocalSystem"));
    }

    #[test]
    fn builtin_service_name_detection() {
        assert!(is_local_service_name("", "LocalService"));
        assert!(is_local_service_name("NT AUTHORITY", "LOCAL SERVICE"));
        assert!(is_network_service_name("", "NetworkService"));
        assert!(is_network_service_name("NT AUTHORITY", "NETWORK SERVICE"));
        assert!(!is_local_service_name("WIN-HOST", "ddagentuser"));
        assert!(!is_local_service_name("CORP", "LocalService"));
        assert!(!is_network_service_name("CORP", "NetworkService"));
    }

    #[test]
    fn well_known_from_names_maps_builtin_accounts() {
        assert_eq!(
            well_known_from_names("", "LocalService"),
            Some(AgentAccount::LocalService)
        );
        assert_eq!(
            well_known_from_names("", "NetworkService"),
            Some(AgentAccount::NetworkService)
        );
        assert_eq!(well_known_from_names("CORP", "gmsa$"), None);
        assert_eq!(well_known_from_names("CORP", "LocalSystem"), None);
        assert_eq!(well_known_from_names("CORP", "LocalService"), None);
    }

    #[test]
    fn agent_password_prefers_installer_lsa_secret() {
        assert_eq!(
            agent_password_from_sources(Some("installer"), Some("scm"), true),
            Some("installer".to_string())
        );
    }

    #[test]
    fn agent_password_falls_back_to_scm_when_installer_lsa_missing() {
        assert_eq!(
            agent_password_from_sources(None, Some("scm"), true),
            Some("scm".to_string())
        );
        assert_eq!(
            agent_password_from_sources(Some(""), Some("scm"), true),
            Some("scm".to_string())
        );
    }

    #[test]
    fn agent_password_ignores_scm_when_service_account_mismatch() {
        assert_eq!(agent_password_from_sources(None, Some("scm"), false), None);
    }

    #[test]
    fn installed_gmsa_takes_precedence_over_stale_lsa_password() {
        let account = agent_account_from_msa_and_lsa(
            "CORP".to_string(),
            "gmsa$".to_string(),
            Some("stale-password-from-previous-ddagentuser"),
            false,
            ManagedServiceAccountState::Installed,
        )
        .expect("installed gMSA should ignore stale LSA password");
        assert_eq!(
            account,
            AgentAccount::ServiceAccountLogon {
                domain: "CORP".to_string(),
                user: "gmsa$".to_string(),
            }
        );
    }

    #[test]
    fn lsa_password_used_when_account_is_not_gmsa() {
        let account = agent_account_from_msa_and_lsa(
            "CORP".to_string(),
            "ddagent".to_string(),
            Some("secret"),
            false,
            ManagedServiceAccountState::NotService,
        )
        .expect("regular domain account should use LSA password");
        assert_eq!(
            account,
            AgentAccount::PasswordLogon {
                domain: "CORP".to_string(),
                user: "ddagent".to_string(),
                password: "secret".to_string(),
            }
        );
    }

    #[test]
    fn lsa_password_used_when_gmsa_classification_unavailable_for_regular_domain_account() {
        let account = agent_account_from_msa_and_lsa(
            "CORP".to_string(),
            "ddagent".to_string(),
            Some("secret"),
            false,
            ManagedServiceAccountState::ClassificationUnavailable,
        )
        .expect("regular domain account should fall back to LSA when DC is unavailable");
        assert_eq!(
            account,
            AgentAccount::PasswordLogon {
                domain: "CORP".to_string(),
                user: "ddagent".to_string(),
                password: "secret".to_string(),
            }
        );
    }

    #[test]
    fn stale_lsa_ignored_when_gmsa_candidate_and_classification_unavailable() {
        let account = agent_account_from_msa_and_lsa(
            "CORP".to_string(),
            "gmsa$".to_string(),
            Some("stale-password-from-previous-ddagentuser"),
            false,
            ManagedServiceAccountState::ClassificationUnavailable,
        )
        .expect("gMSA candidate should not use stale LSA when DC is unavailable");
        assert_eq!(
            account,
            AgentAccount::ServiceAccountLogon {
                domain: "CORP".to_string(),
                user: "gmsa$".to_string(),
            }
        );
    }

    #[test]
    fn should_query_managed_service_account_skips_dc_for_password_backed_domain_accounts() {
        assert!(!should_query_managed_service_account(
            "ddagent",
            Some("secret")
        ));
        assert!(should_query_managed_service_account("ddagent", None));
        assert!(should_query_managed_service_account("ddagent", Some("")));
        assert!(should_query_managed_service_account(
            "gmsa$",
            Some("secret")
        ));
    }

    #[test]
    fn passwordless_local_account_requires_password() {
        let err = passwordless_agent_account(
            String::new(),
            "ddagentuser".to_string(),
            true,
            ManagedServiceAccountState::NotService,
        )
        .expect_err("local accounts without LSA password should fail");
        assert!(
            err.to_string().contains("local account"),
            "unexpected error: {err:#}"
        );
    }

    #[test]
    fn passwordless_gmsa_uses_service_account_logon() {
        let account = passwordless_agent_account(
            "CORP".to_string(),
            "gmsa$".to_string(),
            false,
            ManagedServiceAccountState::Installed,
        )
        .expect("installed gMSA should allow passwordless logon");
        assert_eq!(
            account,
            AgentAccount::ServiceAccountLogon {
                domain: "CORP".to_string(),
                user: "gmsa$".to_string(),
            }
        );
    }

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
        assert_eq!(
            AgentAccount::ServiceAccountLogon {
                domain: "CORP".to_string(),
                user: "gmsa$".to_string(),
            }
            .display_name(),
            AccountName::new("CORP", "gmsa$").display(),
        );
    }

    #[test]
    fn display_name_normalizes_local_machine_domain() {
        let username = "Administrator";
        let sid =
            match lookup_account_sid(".", username).or_else(|_| lookup_account_sid("", username)) {
                Ok(sid) => sid,
                Err(e) => {
                    eprintln!("skipping: built-in Administrator not available: {e:#}");
                    return;
                }
            };
        if !is_local_account(&sid).unwrap_or(false) {
            eprintln!("skipping: Administrator is not a local SAM account on this host");
            return;
        }

        let computer = super::super::local_account::computer_name().expect("computer name");
        assert_eq!(
            AgentAccount::PasswordLogon {
                domain: computer,
                user: username.to_string(),
                password: "secret".to_string(),
            }
            .display_name(),
            AccountName::new("", username).display(),
            "installer machine-name domain should display as .\\user for local SAM accounts"
        );
    }

    #[test]
    fn passwordless_domain_account_requires_password() {
        let err = passwordless_agent_account(
            "CORP".to_string(),
            "ddagent".to_string(),
            false,
            ManagedServiceAccountState::NotService,
        )
        .expect_err("regular domain accounts should require a password");
        assert!(
            err.to_string().contains("password is not available"),
            "unexpected error: {err:#}"
        );
    }

    #[test]
    fn passwordless_domain_account_with_dollar_suffix_requires_valid_gmsa() {
        let err = passwordless_agent_account(
            "CORP".to_string(),
            "notgmsa$".to_string(),
            false,
            ManagedServiceAccountState::NotService,
        )
        .expect_err("accounts ending with $ should be rejected when not gMSA");
        assert!(
            err.to_string().contains("ends with '$'"),
            "unexpected error: {err:#}"
        );
    }
}
