// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"strings"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	gcphost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/gcp/host/linux"
)

type linuxGCPHostnameSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxGCPHostnameSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxGCPHostnameSuite{}, e2e.WithProvisioner(gcphost.ProvisionerNoFakeIntake()))
}

func (v *linuxGCPHostnameSuite) TestAgentConfigHostnameFileOverride() {
	fileContent := "hostname.from.file"
	v.UpdateEnv(gcphost.ProvisionerNoFakeIntake(gcphost.WithAgentOptions(agentparams.WithFile("/tmp/var/hostname", fileContent, false), agentparams.WithAgentConfig("hostname_file: /tmp/var/hostname"))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), fileContent, hostname)
}

func (v *linuxGCPHostnameSuite) TestAgentHostnameCloudProviderDisabled() {
	v.UpdateEnv(gcphost.ProvisionerNoFakeIntake(gcphost.WithAgentOptions(agentparams.WithAgentConfig("cloud_provider_metadata: []"))))

	expected := v.Env().RemoteHost.MustExecute("hostname")
	expected = strings.TrimSpace(expected)

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), expected, hostname)
}

// https://github.com/DataDog/datadog-agent/blob/main/pkg/util/hostname/README.md#the-current-logic
// GCE specific logic from pkg/util/cloudproviders/gce/gce.go
func (v *linuxGCPHostnameSuite) TestAgentGCPHostname() {
	expected := v.Env().RemoteHost.MustExecute(`curl -s -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/instance/hostname`)

	hostname := v.Env().Agent.Client.Hostname()
	v.Equal(expected, hostname)
}
