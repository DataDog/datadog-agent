// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// A later PR removes this allow when spawn reads the translated identity.
#![cfg_attr(not(test), allow(dead_code))]

use std::collections::BTreeSet;
use std::fmt;

use serde::Deserialize;

use crate::config_gate::HostOs;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub(crate) enum RunAs {
    #[default]
    Agent,
    Privileged,
}

// Declaration order is `BTree` order. `BindPrivilegedPorts` is the earlier grant.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub(crate) enum Grant {
    BindPrivilegedPorts,
    RawNetwork,
}

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(try_from = "RawRequestedIdentity")]
pub(crate) struct RequestedIdentity {
    pub(crate) run_as: RunAs,
    pub(crate) grants: BTreeSet<Grant>,
}

#[derive(Deserialize)]
#[serde(
    deny_unknown_fields,
    expecting = "a mapping with optional run_as and grants"
)]
struct RawRequestedIdentity {
    #[serde(default)]
    run_as: RunAs,
    #[serde(default)]
    grants: Vec<Grant>,
}

impl TryFrom<RawRequestedIdentity> for RequestedIdentity {
    type Error = String;

    fn try_from(raw: RawRequestedIdentity) -> Result<Self, Self::Error> {
        let mut grants = BTreeSet::new();
        for grant in raw.grants {
            if !grants.insert(grant) {
                let name = grant_name(grant);
                return Err(format!("identity: duplicate grant {name}"));
            }
        }
        Ok(Self {
            run_as: raw.run_as,
            grants,
        })
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub(crate) enum LinuxCap {
    NetBindService,
    NetRaw,
}

// `Exactly(empty)` drops every capability. It does not leave the daemon's sets in place.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum CapabilityIntent {
    None,
    Exactly(BTreeSet<LinuxCap>),
    KeepDaemon,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum PrivilegedAccount {
    LinuxUid0,
    MacOsRootAdmin,
    WindowsLocalSystem,
}

/// Group selection is not part of this table. `Account::Agent` means the installed
/// account: on Linux its passwd primary and supplementary groups, on macOS group
/// `daemon`, on Windows the existing Agent spawn path. `LinuxUid0` fixes only the
/// uid. The gid and supplementary groups are decided by the PR that applies the account.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Account {
    Agent,
    Privileged(PrivilegedAccount),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct TranslatedIdentity {
    pub(crate) account: Account,
    pub(crate) capabilities: CapabilityIntent,
    pub(crate) satisfied: BTreeSet<Grant>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum TranslateError {
    GrantsWithPrivileged,
    GrantUnavailable { grant: Grant, os: HostOs },
    PrivilegedUnsupported,
}

impl fmt::Display for TranslateError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            TranslateError::GrantsWithPrivileged => {
                write!(f, "identity: grants are not allowed with run_as privileged")
            }
            TranslateError::GrantUnavailable { grant, os } => write!(
                f,
                "identity: grant {} is not available on {}",
                grant_name(*grant),
                host_os_name(*os),
            ),
            TranslateError::PrivilegedUnsupported => write!(
                f,
                "identity: run_as privileged is not available on {}",
                host_os_name(HostOs::Other),
            ),
        }
    }
}

fn grant_name(grant: Grant) -> &'static str {
    match grant {
        Grant::BindPrivilegedPorts => "bind-privileged-ports",
        Grant::RawNetwork => "raw-network",
    }
}

fn host_os_name(os: HostOs) -> &'static str {
    match os {
        HostOs::Linux => "linux",
        HostOs::MacOs => "macos",
        HostOs::Windows => "windows",
        HostOs::Other => "other",
    }
}

pub(crate) fn translate(
    request: &RequestedIdentity,
    os: HostOs,
) -> Result<TranslatedIdentity, TranslateError> {
    if request.run_as == RunAs::Privileged && !request.grants.is_empty() {
        return Err(TranslateError::GrantsWithPrivileged);
    }
    if let Some(grant) = first_unavailable(os, &request.grants) {
        return Err(TranslateError::GrantUnavailable { grant, os });
    }

    Ok(TranslatedIdentity {
        account: account(os, request.run_as)?,
        capabilities: capabilities(os, request.run_as, &request.grants),
        satisfied: satisfied(os, &request.grants),
    })
}

fn first_unavailable(os: HostOs, grants: &BTreeSet<Grant>) -> Option<Grant> {
    grants
        .iter()
        .copied()
        .find(|grant| !grant_available(os, *grant))
}

fn grant_available(os: HostOs, grant: Grant) -> bool {
    match (os, grant) {
        (HostOs::Linux, Grant::BindPrivilegedPorts | Grant::RawNetwork) => true,
        (HostOs::Windows, Grant::BindPrivilegedPorts) => true,
        (HostOs::Windows, Grant::RawNetwork)
        | (HostOs::MacOs | HostOs::Other, Grant::BindPrivilegedPorts | Grant::RawNetwork) => false,
    }
}

fn account(os: HostOs, run_as: RunAs) -> Result<Account, TranslateError> {
    Ok(match (os, run_as) {
        (_, RunAs::Agent) => Account::Agent,
        (HostOs::Linux, RunAs::Privileged) => Account::Privileged(PrivilegedAccount::LinuxUid0),
        (HostOs::MacOs, RunAs::Privileged) => {
            Account::Privileged(PrivilegedAccount::MacOsRootAdmin)
        }
        (HostOs::Windows, RunAs::Privileged) => {
            Account::Privileged(PrivilegedAccount::WindowsLocalSystem)
        }
        (HostOs::Other, RunAs::Privileged) => return Err(TranslateError::PrivilegedUnsupported),
    })
}

fn capabilities(os: HostOs, run_as: RunAs, grants: &BTreeSet<Grant>) -> CapabilityIntent {
    match (os, run_as) {
        (HostOs::Linux, RunAs::Agent) => CapabilityIntent::Exactly(
            grants
                .iter()
                .copied()
                .map(|grant| match grant {
                    Grant::BindPrivilegedPorts => LinuxCap::NetBindService,
                    Grant::RawNetwork => LinuxCap::NetRaw,
                })
                .collect(),
        ),
        (HostOs::Linux, RunAs::Privileged) => CapabilityIntent::KeepDaemon,
        (HostOs::MacOs | HostOs::Windows | HostOs::Other, _) => CapabilityIntent::None,
    }
}

fn satisfied(os: HostOs, grants: &BTreeSet<Grant>) -> BTreeSet<Grant> {
    match os {
        HostOs::Windows => grants
            .iter()
            .copied()
            .filter(|grant| match grant {
                Grant::BindPrivilegedPorts => true,
                Grant::RawNetwork => false,
            })
            .collect(),
        HostOs::Linux | HostOs::MacOs | HostOs::Other => BTreeSet::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn grants(items: &[Grant]) -> BTreeSet<Grant> {
        items.iter().copied().collect()
    }

    fn caps(items: &[LinuxCap]) -> BTreeSet<LinuxCap> {
        items.iter().copied().collect()
    }

    fn request(run_as: RunAs, items: &[Grant]) -> RequestedIdentity {
        RequestedIdentity {
            run_as,
            grants: grants(items),
        }
    }

    #[test]
    fn translate_covers_every_host_os_run_as_and_grant_set() {
        let bind = [Grant::BindPrivilegedPorts];
        let raw = [Grant::RawNetwork];
        let both = [Grant::BindPrivilegedPorts, Grant::RawNetwork];
        let none: [Grant; 0] = [];

        let bind_cap = caps(&[LinuxCap::NetBindService]);
        let raw_cap = caps(&[LinuxCap::NetRaw]);
        let both_caps = caps(&[LinuxCap::NetBindService, LinuxCap::NetRaw]);

        let cases = [
            (
                HostOs::Linux,
                request(RunAs::Agent, &none),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::Exactly(BTreeSet::new()),
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Linux,
                request(RunAs::Agent, &bind),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::Exactly(bind_cap),
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Linux,
                request(RunAs::Agent, &raw),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::Exactly(raw_cap),
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Linux,
                request(RunAs::Agent, &both),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::Exactly(both_caps),
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Linux,
                request(RunAs::Privileged, &none),
                Ok(TranslatedIdentity {
                    account: Account::Privileged(PrivilegedAccount::LinuxUid0),
                    capabilities: CapabilityIntent::KeepDaemon,
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Linux,
                request(RunAs::Privileged, &bind),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Linux,
                request(RunAs::Privileged, &raw),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Linux,
                request(RunAs::Privileged, &both),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Agent, &none),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::None,
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Agent, &bind),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::BindPrivilegedPorts,
                    os: HostOs::MacOs,
                }),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Agent, &raw),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::RawNetwork,
                    os: HostOs::MacOs,
                }),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Agent, &both),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::BindPrivilegedPorts,
                    os: HostOs::MacOs,
                }),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Privileged, &none),
                Ok(TranslatedIdentity {
                    account: Account::Privileged(PrivilegedAccount::MacOsRootAdmin),
                    capabilities: CapabilityIntent::None,
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Privileged, &bind),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Privileged, &raw),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::MacOs,
                request(RunAs::Privileged, &both),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Windows,
                request(RunAs::Agent, &none),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::None,
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Windows,
                request(RunAs::Agent, &bind),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::None,
                    satisfied: grants(&bind),
                }),
            ),
            (
                HostOs::Windows,
                request(RunAs::Agent, &raw),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::RawNetwork,
                    os: HostOs::Windows,
                }),
            ),
            (
                HostOs::Windows,
                request(RunAs::Agent, &both),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::RawNetwork,
                    os: HostOs::Windows,
                }),
            ),
            (
                HostOs::Windows,
                request(RunAs::Privileged, &none),
                Ok(TranslatedIdentity {
                    account: Account::Privileged(PrivilegedAccount::WindowsLocalSystem),
                    capabilities: CapabilityIntent::None,
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Windows,
                request(RunAs::Privileged, &bind),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Windows,
                request(RunAs::Privileged, &raw),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Windows,
                request(RunAs::Privileged, &both),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Other,
                request(RunAs::Agent, &none),
                Ok(TranslatedIdentity {
                    account: Account::Agent,
                    capabilities: CapabilityIntent::None,
                    satisfied: BTreeSet::new(),
                }),
            ),
            (
                HostOs::Other,
                request(RunAs::Agent, &bind),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::BindPrivilegedPorts,
                    os: HostOs::Other,
                }),
            ),
            (
                HostOs::Other,
                request(RunAs::Agent, &raw),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::RawNetwork,
                    os: HostOs::Other,
                }),
            ),
            (
                HostOs::Other,
                request(RunAs::Agent, &both),
                Err(TranslateError::GrantUnavailable {
                    grant: Grant::BindPrivilegedPorts,
                    os: HostOs::Other,
                }),
            ),
            (
                HostOs::Other,
                request(RunAs::Privileged, &none),
                Err(TranslateError::PrivilegedUnsupported),
            ),
            (
                HostOs::Other,
                request(RunAs::Privileged, &bind),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Other,
                request(RunAs::Privileged, &raw),
                Err(TranslateError::GrantsWithPrivileged),
            ),
            (
                HostOs::Other,
                request(RunAs::Privileged, &both),
                Err(TranslateError::GrantsWithPrivileged),
            ),
        ];

        assert_eq!(cases.len(), 32, "every HostOs, run_as, and grant set");
        let mut seen = Vec::new();
        for (os, request, expected) in &cases {
            let key = (*os, request.run_as, request.grants.clone());
            assert!(!seen.contains(&key), "duplicate case");
            seen.push(key);
            assert_eq!(translate(request, *os), expected.clone());
        }
    }

    #[test]
    fn requested_identities_with_grants_inserted_in_opposite_orders_are_equal() {
        let mut bind_then_raw = BTreeSet::new();
        bind_then_raw.insert(Grant::BindPrivilegedPorts);
        bind_then_raw.insert(Grant::RawNetwork);

        let mut raw_then_bind = BTreeSet::new();
        raw_then_bind.insert(Grant::RawNetwork);
        raw_then_bind.insert(Grant::BindPrivilegedPorts);

        assert_eq!(
            RequestedIdentity {
                run_as: RunAs::Agent,
                grants: bind_then_raw,
            },
            RequestedIdentity {
                run_as: RunAs::Agent,
                grants: raw_then_bind,
            },
        );
    }
}
