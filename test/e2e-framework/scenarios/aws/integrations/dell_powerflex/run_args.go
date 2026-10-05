// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dellpowerflex

import "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

const (
	// Pulumi config keys (ddinfra: namespace). All optional:
	//   -c ddinfra:dell_powerflex/goldenAMI=ami-...   (override the golden image)
	//   -c ddinfra:dell_powerflex/goldenAMI=          (empty -> from-scratch build path)
	//   -c ddinfra:dell_powerflex/gatewayUrl=https://... / username=... / password=...
	goldenAMIParam  = "dell_powerflex/goldenAMI"
	gatewayURLParam = "dell_powerflex/gatewayUrl"
	usernameParam   = "dell_powerflex/username"
	passwordParam   = "dell_powerflex/password"
)

// Params holds the run parameters for the Dell PowerFlex scenario.
type Params struct {
	// GoldenAMI is the image booted on the m5.metal host. Defaults to
	// defaultGoldenAMI (us-east-1). Set the goldenAMI config key to an empty
	// string to take the from-scratch build path (vanilla RHEL9 + InstallVirtStack
	// + deferred PFMP bootstrap) instead.
	GoldenAMI string
	// GatewayURL / Username / Password optionally override the embedded check
	// config's powerflex_gateway_url / powerflex_username / powerflex_password
	// (each empty = keep the committed default).
	GatewayURL string
	Username   string
	Password   string
}

// ParamsFromEnvironment builds Params from ddinfra: config, defaulting to the
// turnkey golden AMI and the committed check config.
func ParamsFromEnvironment(e aws.Environment) *Params {
	return &Params{
		GoldenAMI:  e.GetStringWithDefault(e.InfraConfig, goldenAMIParam, defaultGoldenAMI),
		GatewayURL: e.GetStringWithDefault(e.InfraConfig, gatewayURLParam, ""),
		Username:   e.GetStringWithDefault(e.InfraConfig, usernameParam, ""),
		Password:   e.GetStringWithDefault(e.InfraConfig, passwordParam, ""),
	}
}
