// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import "time"

// The types below hold what the job_scheduler_events check of the Cluster
// Agent builds its events from. They are set in the SchedulerEvents field of
// the entities, which is an interface. They are declared here, and not next to
// the entities, so that they are linked in the Cluster Agent only: node Agents
// receive the entities through the kube metadata stream, which does not carry
// this data, and would otherwise pay for the types in the binary.

// KueueWorkloadCondition is a status condition of a Kueue Workload.
type KueueWorkloadCondition struct {
	Type               string
	Status             string
	Reason             string
	Message            string
	LastTransitionTime time.Time
}

// KueueWorkloadEvents is the SchedulerEvents of a KubernetesKueueWorkload.
type KueueWorkloadEvents struct {
	CreationTimestamp time.Time
	// Priority is spec.priority, nil when the Workload does not set one.
	Priority          *int32
	PriorityClassName string
	// Conditions are the status conditions that have a type, in the order
	// Kueue reports them.
	Conditions []KueueWorkloadCondition
}

// KAISchedulingExplanation is one reason KAI gives for not scheduling a PodGroup.
type KAISchedulingExplanation struct {
	Reason  string
	Message string
}

// KAISchedulingCondition is an entry of the status.schedulingConditions of a
// KAI PodGroup. KAI reports why a PodGroup cannot be scheduled in one
// condition per node pool, and removes it once the PodGroup is scheduled.
type KAISchedulingCondition struct {
	Type               string
	Status             string
	NodePool           string
	Reason             string
	Message            string
	Explanations       []KAISchedulingExplanation
	LastTransitionTime time.Time
}

// KAIPodGroupEvents is the SchedulerEvents of a KubernetesKAIPodGroup.
type KAIPodGroupEvents struct {
	CreationTimestamp time.Time
	PriorityClassName string
	// LastStartTime and LastEvictionTime are the times KAI last allocated or
	// evicted the pods of the PodGroup, read from its annotations. They are
	// zero when KAI has not stamped them.
	LastStartTime        time.Time
	LastEvictionTime     time.Time
	SchedulingConditions []KAISchedulingCondition
}
