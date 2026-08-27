// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package haagent

// WorkloadBalancingType is the type value comp/workloadbalancing sets on its own HA_AGENT
// documents. HA Agent's own documents never set type at all. Both comp/haagent and
// comp/workloadbalancing read this from here so the two can't drift apart.
const WorkloadBalancingType = "workload_balancing"

// WorkloadBalancingDiscriminator tells whether an HA_AGENT document belongs to
// comp/workloadbalancing rather than HA Agent.
type WorkloadBalancingDiscriminator struct {
	Type string `json:"type"`
}
