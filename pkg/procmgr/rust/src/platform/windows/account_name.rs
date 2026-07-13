// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

/// Windows `DOMAIN\user` components for operator-facing display.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AccountName {
    domain: String,
    user: String,
}

const NT_AUTHORITY: &str = "NT AUTHORITY";

impl AccountName {
    pub(crate) fn new(domain: impl Into<String>, user: impl Into<String>) -> Self {
        Self {
            domain: domain.into(),
            user: user.into(),
        }
    }

    pub(crate) fn local_system() -> Self {
        Self::new(NT_AUTHORITY, "SYSTEM")
    }

    pub(crate) fn local_service() -> Self {
        Self::new(NT_AUTHORITY, "LocalService")
    }

    pub(crate) fn network_service() -> Self {
        Self::new(NT_AUTHORITY, "NetworkService")
    }

    pub(crate) fn display(&self) -> String {
        if self.domain.is_empty() {
            format!(r".\{}", self.user)
        } else {
            format!("{}\\{}", self.domain, self.user)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn display_formats_well_known_accounts() {
        assert_eq!(
            AccountName::local_system().display(),
            r"NT AUTHORITY\SYSTEM"
        );
        assert_eq!(
            AccountName::local_service().display(),
            r"NT AUTHORITY\LocalService"
        );
        assert_eq!(
            AccountName::network_service().display(),
            r"NT AUTHORITY\NetworkService"
        );
    }

    #[test]
    fn display_formats_local_and_domain_accounts() {
        assert_eq!(
            AccountName::new("", "ddagentuser").display(),
            r".\ddagentuser"
        );
        assert_eq!(AccountName::new("CORP", "gmsa$").display(), r"CORP\gmsa$");
    }
}
