// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package registry

import (
	"maps"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/gke"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/openshiftvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	awsgensimeks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/gensim-eks"
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

var (
	extensionScenarios   = ScenarioRegistry{}
	extensionScenariosMu sync.RWMutex
)

// RegisterScenario registers a scenario by name for use with the run binary.
// Call this from an init() function in a test package's scenario.go; the run
// binary blank-imports that package via scenarios_import_gen.go so the init()
// fires at startup. Regenerate scenarios_import_gen.go by running:
//
//	go generate ./test/new-e2e/run/
func RegisterScenario(name string, fn pulumi.RunFunc) {
	lower := strings.ToLower(name)
	extensionScenariosMu.Lock()
	defer extensionScenariosMu.Unlock()
	if _, exists := builtinScenarios[lower]; exists {
		panic("RegisterScenario: name " + name + " conflicts with a built-in scenario")
	}
	if _, exists := extensionScenarios[lower]; exists {
		panic("RegisterScenario: name " + name + " is already registered")
	}
	extensionScenarios[lower] = fn
}

// builtinScenarioFuncs is the single source of truth for all built-in
// scenarios. Both Scenarios() and RegisterScenario's conflict check derive
// from this map so they can never diverge.
var builtinScenarioFuncs = ScenarioRegistry{
	"aws/vm":          ec2.VMRun,
	"aws/dockervm":    ec2docker.DockerRun,
	"aws/ecs":         ecs.Run,
	"aws/eks":         eks.Run,
	"aws/gensim-eks":  awsgensimeks.Run,
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

// builtinScenarios is the set of built-in keys, derived from builtinScenarioFuncs
// at init time, used by RegisterScenario for O(1) conflict detection.
var builtinScenarios = func() map[string]struct{} {
	s := make(map[string]struct{}, len(builtinScenarioFuncs))
	for k := range builtinScenarioFuncs {
		s[k] = struct{}{}
	}
	return s
}()

func Scenarios() ScenarioRegistry {
	combined := make(ScenarioRegistry, len(builtinScenarioFuncs))
	maps.Copy(combined, builtinScenarioFuncs)
	extensionScenariosMu.RLock()
	maps.Copy(combined, extensionScenarios)
	extensionScenariosMu.RUnlock()
	return combined
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
