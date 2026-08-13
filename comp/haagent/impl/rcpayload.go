// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

type haAgentConfig struct {
	ConfigID    string `json:"config_id"`
	ActiveAgent string `json:"active_agent"`
}

// workloadBalancingDiscriminator is a minimal envelope used only to tell whether an HA_AGENT
// Remote Config document belongs to comp/workloadbalancing, a second, independent listener that
// shares this product, rather than to HA Agent itself. comp/workloadbalancing's documents set
// group_id; HA Agent's own documents never do.
//
// The json tag below is the entire contract between this struct and comp/workloadbalancing's own
// payload struct (a separate type in a separate package, deliberately not shared, to avoid a
// dependency between the two components). There is nothing enforcing that the two agree on this
// key at compile time — if comp/workloadbalancing's payload ever renames or stops setting
// group_id, this struct must be updated to match, or documents silently stop being routed to
// their owner.
type workloadBalancingDiscriminator struct {
	GroupID string `json:"group_id"`
}
