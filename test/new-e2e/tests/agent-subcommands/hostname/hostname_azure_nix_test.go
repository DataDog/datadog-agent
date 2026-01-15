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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	azurehost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/azure/host/linux"
)

// azureMetadataAPIVersion is the default Azure IMDS API version used by the agent.
const azureMetadataAPIVersion = "api-version=2021-02-01"

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

	metadataStr := v.Env().RemoteHost.MustExecute("curl -s -H \"Metadata: true\" http://169.254.169.254/metadata/instance/compute?" + azureMetadataAPIVersion)

	var metadata struct {
		VMID              string
		Name              string
		ResourceGroupName string
		SubscriptionID    string
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
		// the machine hostname will be used if the style is invalid
		"some_invalid_value": hostname,
	}

	for hostnameStyle, expected := range hostnameStyles {
		v.Run("hostname_style_"+hostnameStyle, func() {
			agentConfig := ""
			if hostnameStyle != "" {
				agentConfig = "azure_hostname_style: " + hostnameStyle
			}

			v.UpdateEnv(azurehost.ProvisionerNoFakeIntake(azurehost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))))

			// Use Eventually to handle transient IMDS availability issues after agent restart.
			// The agent's cachedfetch.Fetcher may momentarily return a stale/fallback hostname
			// if IMDS is slow to respond immediately after restart.
			assert.Eventually(v.T(), func() bool {
				hostname := v.Env().Agent.Client.Hostname()
				return hostname == expected
			}, 30*time.Second, 2*time.Second, "expected hostname %q but got different value", expected)
		})
	}
}
