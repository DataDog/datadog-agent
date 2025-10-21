package registry

import (
	"strings"

	"github.com/DataDog/test-infra-definitions/scenarios/gcp/gke"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/openshiftvm"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/installer"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/kindvm"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/microVMs/microvms"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/aks"
	computerun "github.com/DataDog/test-infra-definitions/scenarios/azure/compute/run"
	gcpcompute "github.com/DataDog/test-infra-definitions/scenarios/gcp/compute/run"
	localpodmanrun "github.com/DataDog/test-infra-definitions/scenarios/local/podman/run"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ScenarioRegistry map[string]pulumi.RunFunc

func Scenarios() ScenarioRegistry {
	return ScenarioRegistry{
		"aws/vm":          ec2.VMRun,
		"aws/dockervm":    ec2.VMRunWithDocker,
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
