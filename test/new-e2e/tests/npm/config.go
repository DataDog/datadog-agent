// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package npm for all NPM new E2E tests
package npm

import (
	_ "embed"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
)

// systemProbeConfigNPM define the embedded minimal configuration for NPM
//
//go:embed config/npm.yaml
var systemProbeConfigNPM string

// systemProbeConfigNPMHelmValues define the embedded minimal configuration for NPM
//
//go:embed config/npm-helm-values.yaml
var systemProbeConfigNPMHelmValues string

// systemProbeConfigNPMEnv equivalent of config/npm.yaml
func systemProbeConfigNPMEnv() []dockeragentparams.Option {
	return []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_NETWORK_ENABLED", pulumi.StringPtr("true")),
	}
}
