// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

type haAgentConfig struct {
	ConfigID    string `json:"config_id"`
	ActiveAgent string `json:"active_agent"`
}

// workloadBalancingDiscriminator tells whether an HA_AGENT document belongs to
// comp/workloadbalancing rather than HA Agent. Its json tag is an unenforced contract with
// comp/workloadbalancing's own payload struct (kept separate to avoid a dependency between the
// two) — keep them in sync by hand.
type workloadBalancingDiscriminator struct {
	GroupID string `json:"group_id"`
}
