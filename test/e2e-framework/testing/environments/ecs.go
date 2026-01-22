// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	ecsscenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// ECS is an environment that contains a ECS deployed in a cluster, FakeIntake and Agent configured to talk to each other.
type ECS struct {
	// Components
	ECSCluster *components.ECSCluster
	FakeIntake *components.FakeIntake
}

// Ensure ECS implements the ECSOutputs interface
var _ ecsscenario.ECSOutputs = (*ECS)(nil)

// ECSClusterOutput implements ecsscenario.ECSOutputs
func (e *ECS) ECSClusterOutput() *ecs.ClusterOutput {
	if e.ECSCluster == nil {
		e.ECSCluster = &components.ECSCluster{}
	}
	return &e.ECSCluster.ClusterOutput
}

// FakeIntakeOutput implements ecsscenario.ECSOutputs
func (e *ECS) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	if e.FakeIntake == nil {
		e.FakeIntake = &components.FakeIntake{}
	}
	return &e.FakeIntake.FakeintakeOutput
}

// DisableFakeIntake implements ecsscenario.ECSOutputs
func (e *ECS) DisableFakeIntake() {
	e.FakeIntake = nil
}
