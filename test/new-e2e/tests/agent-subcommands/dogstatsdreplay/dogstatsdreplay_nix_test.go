// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogstatsdreplay contains e2e tests for the dogstatsd-replay command
package dogstatsdreplay

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type dogstatsdreplayNixTest struct {
	baseDogstatsdReplaySuite
}

// TestLinuxDogstatsdReplaySuite runs the dogstatsd-replay e2e tests on Linux
func TestLinuxDogstatsdReplaySuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dogstatsdreplayNixTest{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(`
log_level: DEBUG
dogstatsd_non_local_traffic: true
dogstatsd_tag_cardinality: high
dogstatsd_origin_detection: true
`),
				),
			),
		)))
}
