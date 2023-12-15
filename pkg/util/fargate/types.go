// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fargate implements functions to interact with fargate
package fargate

// OrchestratorName covers the possible platform names where Fargate can run
type OrchestratorName string

const (
	// ECS represents AWS ECS
	ECS OrchestratorName = "ECS"
	// EKS represents AWS EKS
	EKS OrchestratorName = "EKS"
	// Unknown is used when we cannot retrieve the orchestrator
	Unknown OrchestratorName = "Unknown"
)
