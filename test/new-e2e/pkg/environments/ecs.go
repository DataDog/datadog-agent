// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// ECS is an environment that contains a ECS deployed in a cluster, FakeIntake and Agent configured to talk to each other.
type ECS struct {
	// Components
	ECSCluster *components.ECSCluster
	FakeIntake *components.FakeIntake
}
