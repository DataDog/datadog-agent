// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Kubernetes is an environment that contains a Kubernetes cluster, the Agent and a FakeIntake.
type Kubernetes struct {
	// Components
	KubernetesCluster *components.KubernetesCluster
	FakeIntake        *components.FakeIntake
	Agent             *components.KubernetesAgent
}
