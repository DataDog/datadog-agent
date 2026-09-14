// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{bail, Result};

/// Agent-profile logon as the installed Agent user when the supervisor token is not that user.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AgentPasswordLogon {
    pub registry_domain: String,
    pub logon_domain: String,
    pub user: String,
    pub password: String,
}

/// Resolve a password logon for Agent-profile spawn.
///
/// Local vs domain only selects the Win32 logon domain (`.` vs `CORP`). Both use the
/// installer-stored password. Domain accounts are not a separate unsupported path.
pub(crate) fn resolve_agent_password_logon(
    registry_domain: &str,
    user: &str,
    is_local: bool,
    password: Option<&str>,
) -> Result<AgentPasswordLogon> {
    let display = account_display(registry_domain, user);
    let Some(password) = password.filter(|password| !password.is_empty()) else {
        bail!(
            "agent user password is not available for {display}; \
             ensure the installer stored L$datadog_ddagentuser_password or run \
             dd-procmgrd as the installed agent account"
        );
    };

    Ok(AgentPasswordLogon {
        registry_domain: registry_domain.to_string(),
        logon_domain: logon_domain(registry_domain, is_local),
        user: user.to_string(),
        password: password.to_string(),
    })
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
    fn domain_agent_user_with_password_uses_password_logon() {
        let logon = resolve_agent_password_logon("CORP", "ddagentuser", false, Some("secret"))
            .expect("domain Agent user with an installer password must log on as that user");

        assert_eq!(logon.registry_domain, "CORP");
        assert_eq!(logon.logon_domain, "CORP");
        assert_eq!(logon.user, "ddagentuser");
        assert_eq!(logon.password, "secret");
    }

    #[test]
    fn local_agent_user_with_password_uses_password_logon() {
        let logon = resolve_agent_password_logon("WIN-HOST", "ddagentuser", true, Some("secret"))
            .expect("local Agent user with an installer password must log on as that user");

        assert_eq!(logon.registry_domain, "WIN-HOST");
        assert_eq!(logon.logon_domain, "");
        assert_eq!(logon.user, "ddagentuser");
        assert_eq!(logon.password, "secret");
    }

    #[test]
    fn domain_agent_user_without_password_does_not_claim_unsupported() {
        let err = resolve_agent_password_logon("CORP", "ddagentuser", false, None)
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
