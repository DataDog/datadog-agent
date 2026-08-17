// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

// workloadBalancingType is the type value comp/workloadbalancing sets on its own HA_AGENT
// documents. HA Agent's own documents never set type at all.
const workloadBalancingType = "workload_balancing"

// workloadBalancingDiscriminator tells whether an HA_AGENT document belongs to
// comp/workloadbalancing. Its json tag must match comp/haagent's own copy by hand; the two are
// deliberately not shared to avoid a dependency between the components.
type workloadBalancingDiscriminator struct {
	Type string `json:"type"`
}

// workloadBalancingRCConfig is the workload-balancing payload on the shared HA_AGENT product.
// Named workload_balancing_group_id, not group/group_id/GroupID, to avoid colliding with
// ha_agent.group, a dormant config key and RC schema field reserved for an unrelated,
// in-flight proposal to make agent_group the canonical HA cluster identity on this product.
type workloadBalancingRCConfig struct {
	WorkloadBalancingGroupID string `json:"workload_balancing_group_id"`
	ActiveAgent              string `json:"active_agent"`
}
