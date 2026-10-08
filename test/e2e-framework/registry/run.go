// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package registry

import (
	"fmt"
	"os"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	scenarioEnvVarName = "PULUMI_SCENARIO"
	scenarioParamName  = "scenario"

	dummyScenario = "dummy"
)

// Run is the shared Pulumi entry point for every scenario runner binary. A
// runner's main() is expected to be nothing more than a call to Run; the only
// thing that distinguishes one runner from another is the set of packages it
// blank-imports to populate the registry via RegisterScenario.
func Run() {
	pulumi.Run(ScenarioRunFunc())
}

// ScenarioRunFunc resolves the scenario to run from the PULUMI_SCENARIO
// environment variable, overridden by the `scenario` stack config value, and
// dispatches to the matching registered scenario.
func ScenarioRunFunc() pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		scenarioName := os.Getenv(scenarioEnvVarName)
		rootConfig := config.New(ctx, "")
		if s := rootConfig.Get(scenarioParamName); s != "" {
			scenarioName = s
		}

		// Fake stack name used to pre-download pulumi plugins due to a bug with `pulumi plugin install` and azure-native-sdk
		if scenarioName == dummyScenario {
			return nil
		}

		rf := Scenarios().Get(scenarioName)
		if rf == nil {
			return fmt.Errorf("impossible to run unknown scenario: %s, known scenarios: %s", scenarioName, strings.Join(Scenarios().List(), " ,"))
		}

		return rf(ctx)
	}
}
