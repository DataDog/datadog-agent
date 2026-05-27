// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package interfaceresolver maps observed network interfaces (e.g. from
// SNMP IF-MIB, NetFlow exporters, or host telemetry) to a canonical
// interface identity.
//
// The legacy resolver keyed primarily on MAC address. This breaks when
// multiple logical interfaces legitimately share a MAC with their parent
// physical interface — most commonly VLAN sub-interfaces (IF-MIB
// ifPhysAddress is not unique across ifIndex when sub-interfaces inherit
// the parent MAC).
//
// V1 of this package introduces a deterministic tiebreaker using
// IF-MIB ifType plus a small amount of resolution context (an optional
// expected-type hint and an optional interface-name hint). The exact
// priority of rules is documented on Resolver.Resolve.
//
// Design boundaries for V1:
//
//   - Only ifType + lightweight context. No LLDP/CDP/ifStackTable fusion;
//     those are deferred until the audit metric (see Auditor) tells us
//     ifType alone is not enough.
//   - The package exposes a small Observer interface so callers can wire
//     in the agent's preferred telemetry backend without this package
//     having to depend on it.
//   - Forward-only. The package does not re-resolve historical telemetry.
//   - Does not change the canonical interface ID downstream; tagging
//     contract stays the same — only the resolver's accuracy improves.
package interfaceresolver
