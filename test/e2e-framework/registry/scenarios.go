// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package registry

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/gke"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/openshiftvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/microvms"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/aks"
	computerun "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/compute/run"
	gcpcompute "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/compute/run"
	localpodmanrun "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/local/podman/run"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ScenarioRegistry map[string]pulumi.RunFunc

func Scenarios() ScenarioRegistry {
	return ScenarioRegistry{
		"aws/vm":          ec2.VMRun,
		"aws/dockervm":    ec2docker.DockerRun,
		"aws/ecs":         ecs.Run,
		"aws/eks":         eks.Run,
		"aws/installer":   installer.Run,
		"aws/microvms":    microvms.Run,
		"aws/kind":        kindvm.Run,
		"az/vm":           computerun.VMRun,
		"az/aks":          aks.Run,
		"gcp/vm":          gcpcompute.VMRun,
		"gcp/gke":         gke.Run,
		"gcp/openshiftvm": openshiftvm.Run,
		"localpodman/vm":  localpodmanrun.VMRun,
	}
}

func (s ScenarioRegistry) Get(name string) pulumi.RunFunc {
	return s[strings.ToLower(name)]
}

func (s ScenarioRegistry) List() []string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}

	return names
}
