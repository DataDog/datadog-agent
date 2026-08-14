// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

type haAgentConfig struct {
	ConfigID    string `json:"config_id"`
	ActiveAgent string `json:"active_agent"`
}

// workloadBalancingType is the discriminator value comp/workloadbalancing sets on its own
// HA_AGENT documents. HA Agent's own documents never set type at all.
const workloadBalancingType = "workload_balancing"

// workloadBalancingDiscriminator tells whether an HA_AGENT document belongs to
// comp/workloadbalancing rather than HA Agent. Its json tag must match that package's own
// payload struct by hand; the two are deliberately not shared to avoid a dependency.
type workloadBalancingDiscriminator struct {
	Type string `json:"type"`
}
