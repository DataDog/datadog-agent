// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};

/// Installed Agent user identity for Agent-profile spawn.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AgentLogonIdentity {
    pub registry_domain: String,
    pub logon_domain: String,
    pub user: String,
}

/// Resolved Agent-profile logon when dd-procmgrd does not run as the installed user.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum AgentSpawnLogon {
    InstallerPassword(AgentLogonIdentity),
    ManagedServiceAccount(AgentLogonIdentity),
}

/// Resolve Agent-profile logon for LocalSystem procmgr.
///
/// Local vs domain only selects the Win32 logon domain (`.` vs `CORP`). Managed service
/// accounts win over a stale installer LSA secret when MSI secret removal was best-effort.
pub(crate) fn resolve_agent_spawn_logon(
    registry_domain: &str,
    user: &str,
    is_local: bool,
    installer_password_present: bool,
    is_managed_service_account: bool,
) -> Result<AgentSpawnLogon> {
    if is_managed_service_account {
        return Ok(AgentSpawnLogon::ManagedServiceAccount(
            agent_logon_identity(registry_domain, user, is_local),
        ));
    }

    if installer_password_present {
        return Ok(AgentSpawnLogon::InstallerPassword(agent_logon_identity(
            registry_domain,
            user,
            is_local,
        )));
    }

    let display = account_display(registry_domain, user);
    bail!(
        "agent user password is not available for {display}; \
         ensure the installer stored L$datadog_ddagentuser_password or run \
         dd-procmgrd as the installed agent account"
    );
}

fn agent_logon_identity(registry_domain: &str, user: &str, is_local: bool) -> AgentLogonIdentity {
    AgentLogonIdentity {
        registry_domain: registry_domain.to_string(),
        logon_domain: logon_domain(registry_domain, is_local),
        user: user.to_string(),
    }
}

fn account_display(registry_domain: &str, user: &str) -> String {
    if registry_domain.is_empty() {
        format!(r".\{user}")
    } else {
        format!(r"{registry_domain}\{user}")
    }
}

fn logon_domain(registry_domain: &str, is_local: bool) -> String {
    if is_local {
        String::new()
    } else {
        registry_domain.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn domain_agent_user_with_installer_password_uses_password_logon() {
        let logon = resolve_agent_spawn_logon("CORP", "ddagentuser", false, true, false)
            .expect("domain Agent user with an installer password must log on as that user");

        assert_eq!(
            logon,
            AgentSpawnLogon::InstallerPassword(AgentLogonIdentity {
                registry_domain: "CORP".to_string(),
                logon_domain: "CORP".to_string(),
                user: "ddagentuser".to_string(),
            })
        );
    }

    #[test]
    fn local_agent_user_with_installer_password_uses_password_logon() {
        let logon = resolve_agent_spawn_logon("WIN-HOST", "ddagentuser", true, true, false)
            .expect("local Agent user with an installer password must log on as that user");

        assert_eq!(
            logon,
            AgentSpawnLogon::InstallerPassword(AgentLogonIdentity {
                registry_domain: "WIN-HOST".to_string(),
                logon_domain: String::new(),
                user: "ddagentuser".to_string(),
            })
        );
    }

    #[test]
    fn managed_service_account_without_password_uses_passwordless_logon() {
        let logon = resolve_agent_spawn_logon("CORP", "ddgmsa$", false, false, true)
            .expect("gMSA must log on without an installer password");

        assert_eq!(
            logon,
            AgentSpawnLogon::ManagedServiceAccount(AgentLogonIdentity {
                registry_domain: "CORP".to_string(),
                logon_domain: "CORP".to_string(),
                user: "ddgmsa$".to_string(),
            })
        );
    }

    #[test]
    fn managed_service_account_with_stale_installer_password_uses_passwordless_logon() {
        let logon = resolve_agent_spawn_logon("CORP", "ddgmsa$", false, true, true)
            .expect("gMSA must ignore a stale installer LSA secret");

        assert_eq!(
            logon,
            AgentSpawnLogon::ManagedServiceAccount(AgentLogonIdentity {
                registry_domain: "CORP".to_string(),
                logon_domain: "CORP".to_string(),
                user: "ddgmsa$".to_string(),
            })
        );
    }

    #[test]
    fn domain_agent_user_without_password_does_not_claim_unsupported() {
        let err = resolve_agent_spawn_logon("CORP", "ddagentuser", false, false, false)
            .expect_err("missing password must fail");
        let message = format!("{err:#}");
        assert!(
            !message.contains("not supported"),
            "domain Agent users are supported via password logon; got {message}"
        );
        assert!(
            message.contains("password is not available"),
            "expected a missing-password error; got {message}"
        );
    }
}
