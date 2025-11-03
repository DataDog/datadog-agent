// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/registry"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	scenarioEnvVarName = "PULUMI_SCENARIO"
	scenarioParamName  = "scenario"

	dummyScenario = "dummy"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		scenarioName := os.Getenv(scenarioEnvVarName)
		rootConfig := config.New(ctx, "")
		if s := rootConfig.Get(scenarioParamName); s != "" {
			scenarioName = s
		}

		// Fake stack name used to pre-download pulumi plugins due to a bug with `pulumi plugin install` and azure-native-sdk
		if scenarioName == dummyScenario {
			return nil
		}

		rf := registry.Scenarios().Get(scenarioName)
		if rf == nil {
			return fmt.Errorf("impossible to run unknown scenario: %s, known scenarios: %s", scenarioName, strings.Join(registry.Scenarios().List(), " ,"))
		}

		return rf(ctx)
	})
}
