// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package outputs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
)

// ECSOutputs is the interface for ECS environment outputs.
type ECSOutputs interface {
	ECSClusterOutput() *ecs.ClusterOutput
	FakeIntakeOutput() *fakeintake.FakeintakeOutput
	DisableFakeIntake()
}

// ECS contains the outputs for an ECS environment.
type ECS struct {
	ECSCluster *ecs.ClusterOutput
	FakeIntake *fakeintake.FakeintakeOutput
}

// NewECS creates a new ECS output struct with all fields initialized.
func NewECS() *ECS {
	return &ECS{
		ECSCluster: &ecs.ClusterOutput{},
		FakeIntake: &fakeintake.FakeintakeOutput{},
	}
}

// ECSClusterOutput returns the ECS cluster output for exporting
func (e *ECS) ECSClusterOutput() *ecs.ClusterOutput {
	return e.ECSCluster
}

// FakeIntakeOutput returns the fakeintake output for exporting (may be nil)
func (e *ECS) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	return e.FakeIntake
}

// DisableFakeIntake marks FakeIntake as not provisioned (sets to nil)
func (e *ECS) DisableFakeIntake() {
	e.FakeIntake = nil
}
