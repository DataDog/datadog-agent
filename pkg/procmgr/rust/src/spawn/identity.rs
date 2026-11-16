// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// A later PR removes this allow when something calls `translate`.
#![cfg_attr(not(test), allow(dead_code))]

use std::collections::BTreeSet;

use crate::config_gate::HostOs;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RunAs {
    Agent,
    Privileged,
}

// Declaration order is `BTree` order. `BindPrivilegedPorts` is the earlier grant.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub(crate) enum Grant {
    BindPrivilegedPorts,
    RawNetwork,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct RequestedIdentity {
    pub(crate) run_as: RunAs,
    pub(crate) grants: BTreeSet<Grant>,
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

pub(crate) fn translate(
    request: &RequestedIdentity,
    os: HostOs,
) -> Result<TranslatedIdentity, TranslateError> {
    if request.run_as == RunAs::Privileged && !request.grants.is_empty() {
        return Err(TranslateError::GrantsWithPrivileged);
    }

    match (os, request.run_as) {
        (HostOs::Linux, RunAs::Agent) => {
            let capabilities = request
                .grants
                .iter()
                .copied()
                .map(|grant| match grant {
                    Grant::BindPrivilegedPorts => LinuxCap::NetBindService,
                    Grant::RawNetwork => LinuxCap::NetRaw,
                })
                .collect();
            Ok(TranslatedIdentity {
                account: Account::Agent,
                capabilities: CapabilityIntent::Exactly(capabilities),
                satisfied: BTreeSet::new(),
            })
        }
        (HostOs::Linux, RunAs::Privileged) => Ok(TranslatedIdentity {
            account: Account::Privileged(PrivilegedAccount::LinuxUid0),
            capabilities: CapabilityIntent::KeepDaemon,
            satisfied: BTreeSet::new(),
        }),
        (HostOs::MacOs | HostOs::Other, RunAs::Agent) => {
            if let Some(grant) = request.grants.iter().copied().next() {
                return Err(TranslateError::GrantUnavailable { grant, os });
            }
            Ok(TranslatedIdentity {
                account: Account::Agent,
                capabilities: CapabilityIntent::None,
                satisfied: BTreeSet::new(),
            })
        }
        (HostOs::MacOs, RunAs::Privileged) => Ok(TranslatedIdentity {
            account: Account::Privileged(PrivilegedAccount::MacOsRootAdmin),
            capabilities: CapabilityIntent::None,
            satisfied: BTreeSet::new(),
        }),
        (HostOs::Windows, RunAs::Agent) => {
            let mut satisfied = BTreeSet::new();
            for grant in &request.grants {
                match grant {
                    Grant::BindPrivilegedPorts => {
                        satisfied.insert(*grant);
                    }
                    Grant::RawNetwork => {
                        return Err(TranslateError::GrantUnavailable {
                            grant: *grant,
                            os: HostOs::Windows,
                        });
                    }
                }
            }
            Ok(TranslatedIdentity {
                account: Account::Agent,
                capabilities: CapabilityIntent::None,
                satisfied,
            })
        }
        (HostOs::Windows, RunAs::Privileged) => Ok(TranslatedIdentity {
            account: Account::Privileged(PrivilegedAccount::WindowsLocalSystem),
            capabilities: CapabilityIntent::None,
            satisfied: BTreeSet::new(),
        }),
        (HostOs::Other, RunAs::Privileged) => Err(TranslateError::PrivilegedUnsupported),
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
