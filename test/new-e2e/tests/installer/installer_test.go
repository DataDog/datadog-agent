// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains tests for the datadog installer
package installer

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type packageTests func(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite

var (
	amd64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		// e2eos.AmazonLinux2,
		// e2eos.Debian12,
		// e2eos.RedHat9,
		// e2eos.Suse15,
		// e2eos.Fedora37,
		// e2eos.CentOS7,
	}
	arm64Flavors = []e2eos.Descriptor{
		// e2eos.Ubuntu2204,
		// e2eos.AmazonLinux2,
		// e2eos.Suse15,
	}
	packagesTests = []packageTests{
		testInstaller,
	}
)

func TestPackages(t *testing.T) {
	var flavors []e2eos.Descriptor
	for _, flavor := range amd64Flavors {
		flavor.Architecture = e2eos.AMD64Arch
		flavors = append(flavors, flavor)
	}
	for _, flavor := range arm64Flavors {
		flavor.Architecture = e2eos.ARM64Arch
		flavors = append(flavors, flavor)
	}
	for _, flavor := range flavors {
		for _, test := range packagesTests {
			suite := test(flavor, flavor.Architecture)
			t.Run(suite.Name(), func(t *testing.T) {
				t.Parallel()
				e2e.Run(t, suite,
					e2e.WithProvisioner(
						awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOSArch(flavor, flavor.Architecture)), awshost.WithoutAgent()),
					),
					e2e.WithStackName(suite.Name()),
				)
			})
		}
	}
}
