// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	azurehost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/azure/host/linux"
)

type linuxAzureHostnameSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxAzureHostnameSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxAzureHostnameSuite{}, e2e.WithProvisioner(azurehost.ProvisionerNoFakeIntake()))
}

func (v *linuxAzureHostnameSuite) TestAgentConfigHostnameFileOverride() {
	fileContent := "hostname.from.file"
	v.UpdateEnv(azurehost.ProvisionerNoFakeIntake(azurehost.WithAgentOptions(agentparams.WithFile("/tmp/var/hostname", fileContent, false), agentparams.WithAgentConfig("hostname_file: /tmp/var/hostname"))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), fileContent, hostname)
}

// https://github.com/DataDog/datadog-agent/blob/main/pkg/util/hostname/README.md#the-current-logic
// azure specific logic from pkg/util/cloudproviders/azure/azure.go
func (v *linuxAzureHostnameSuite) TestAgentHostnameStyle() {
	hostname := v.Env().RemoteHost.MustExecute("hostname")
	hostname = strings.TrimSpace(hostname)

	metadataStr := v.Env().RemoteHost.MustExecute(`curl -s -H "Metadata: true" http://169.254.169.254/metadata/instance/compute?api-version=2021-02-01`)

	var metadata struct {
		VMID              string
		Name              string
		ResourceGroupName string
		SubscriptionID    string
		OsProfile         struct {
			ComputerName string
		}
	}

	err := json.Unmarshal([]byte(metadataStr), &metadata)
	require.NoError(v.T(), err)

	hostnameStyles := map[string]interface{}{
		"":                        hostname,
		"os":                      hostname,
		"vmid":                    metadata.VMID,
		"name":                    metadata.Name,
		"name_and_resource_group": fmt.Sprintf("%s.%s", metadata.Name, metadata.ResourceGroupName),
		"full":                    fmt.Sprintf("%s.%s.%s", metadata.Name, metadata.ResourceGroupName, metadata.SubscriptionID),
		"os_computer_name":        strings.ToLower(metadata.OsProfile.ComputerName),
	}

	for hostnameStyle, expected := range hostnameStyles {
		v.Run("hostname_style_"+hostnameStyle, func() {
			agentConfig := ""
			if hostnameStyle != "" {
				agentConfig = "azure_hostname_style: " + hostnameStyle
			}

			v.UpdateEnv(azurehost.ProvisionerNoFakeIntake(azurehost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))))

			hostname := v.Env().Agent.Client.Hostname()
			v.Equal(expected, hostname)
		})
	}
}
