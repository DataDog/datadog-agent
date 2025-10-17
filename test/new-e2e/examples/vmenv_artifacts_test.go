// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"

	"github.com/stretchr/testify/assert"
)

type artifactsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestArtifactsSuite(t *testing.T) {
	e2e.Run(t, &artifactsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoAgentNoFakeIntake(),
	))
}

func (v *artifactsSuite) TestDownloadArtifact() {
	// Download the artifact from S3 bucket to the remote host
	err := v.Env().RemoteHost.HostArtifactClient.Get("toto.txt", "toto.txt")
	assert.NoError(v.T(), err)

	// Read and verify the content of the downloaded file
	out := v.Env().RemoteHost.MustExecute("cat toto.txt")
	assert.Contains(v.T(), out, "hello")
}
