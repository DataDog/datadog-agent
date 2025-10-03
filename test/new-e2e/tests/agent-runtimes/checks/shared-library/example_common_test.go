// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for the rtloader
package sharedlibrary

import (
	_ "embed"
	"path/filepath"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	osVM "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	checkutils "github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

//go:embed files/conf.yaml
var exampleCheckYaml string

type baseSharedLibrarySuite struct {
	e2e.BaseSuite[environments.Host]

	libName      string
	targetFolder string
}

func (v *baseSharedLibrarySuite) getSuiteOptions(osInstance osVM.Descriptor) []e2e.SuiteOption {
	var suiteOptions []e2e.SuiteOption
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithIntegration("example.d", exampleCheckYaml),
			),
			awshost.WithEC2InstanceOptions(ec2.WithOS(osInstance)),
		),
	))

	return suiteOptions
}

func (v *baseSharedLibrarySuite) TestSharedLibraryImplementation() {
	v.T().Log("Running Shared Library Check Example test")

	v.Env().RemoteHost.CopyFile(
		filepath.Join(".", "files", v.libName),
		filepath.Join("/", "tmp", v.libName),
	)

	v.Env().RemoteHost.MustExecute("sudo cp " + filepath.Join("/", "tmp", v.libName) + " " + filepath.Join(v.targetFolder, v.libName))

	// Fetch the check status and metrics in JSON format
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"example", "--json"}))
	data := checkutils.ParseJSONOutput(v.T(), []byte(check))
	metrics := data[0].Aggregator.Metrics
	assert.Equal(v.T(), len(metrics), 1)

	// Check the metric fields
	metric := metrics[0]
	assert.Equal(v.T(), "hello.gauge", metric.Metric)
	assert.Equal(v.T(), metric.Points[0][1], 1.0)
}
