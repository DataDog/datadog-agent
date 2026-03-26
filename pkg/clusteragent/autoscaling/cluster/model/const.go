// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package model

const (
	// SuccessfulNodepoolCreateEventReason is the event reason when a nodepool is created successfully
	SuccessfulNodepoolCreateEventReason = "SuccessfulNodepoolCreate"
	// SuccessfulNodepoolUpdateEventReason is the event reason when a nodepool is updated successfully
	SuccessfulNodepoolUpdateEventReason = "SuccessfulNodepoolUpdate"
	// FailedNodepoolUpdateEventReason is the event reason when a nodepool update fails
	FailedNodepoolUpdateEventReason = "FailedNodepoolUpdate"
	// SuccessfulNodepoolDeleteEventReason is the event reason when a nodepool is deleted successfully
	SuccessfulNodepoolDeleteEventReason = "SuccessfulNodepoolDelete"
	// FailedNodepoolDeleteEventReason is the event reason when a nodepool deletion fails
	FailedNodepoolDeleteEventReason = "FailedNodepoolDelete"
)
