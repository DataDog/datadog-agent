// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname contains helpers and e2e tests of the flare command
package hostname

import (
	"strings"
	"testing"

	"encoding/json"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type Meta struct {
	Hostname                  string   `json:"hostname"`
	HostAliases               []string `json:"host_aliases"`
	InstanceID                string   `json:"instance-id"`
	LegacyResolutionHostname  string   `json:"legacy-resolution-hostname,omitempty"`
	HostnameResolutionVersion int      `json:"hostname-resolution-version,omitempty"`
}

type baseHostnameSuite struct {
	e2e.BaseSuite[environments.Host]

	hostnameMetadata Meta
}

func TestBaseHostnameSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t, &baseHostnameSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func requestAgentHostnameMetadataPayload(v *baseHostnameSuite) {
	v.T().Helper()

	statusArgs := agentclient.WithArgs([]string{"header", "--json"})
	statusPayload := v.Env().Agent.Client.Status(statusArgs)

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(statusPayload.Content), &data); err != nil {
		v.T().Fatal(err)
	}

	metaData, err := json.Marshal(data["metadata"].(map[string]interface{})["meta"])
	if err != nil {
		v.T().Fatal(err)
	}

	if err := json.Unmarshal(metaData, &v.hostnameMetadata); err != nil {
		v.T().Fatal(err)
	}
}

func retrieveMetadata(v *baseHostnameSuite) (string, string) {
	metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)
	instanceID := metadata.Get("instance-id")
	osHostname := strings.Split(metadata.Get("hostname"), ".")[0]
	return instanceID, osHostname
}

func runHostnameTest(v *baseHostnameSuite, instanceOpts []awshost.ProvisionerOption, tt struct {
	name                 string
	agentConfig          string
	imdsEnabled          bool
	expectLegacyHostname bool
}) {
	v.T().Run(tt.name, func(t *testing.T) {
		v.UpdateEnv(awshost.ProvisionerNoFakeIntake(instanceOpts...))
		requestAgentHostnameMetadataPayload(v)

		instanceID, osHostname := retrieveMetadata(v)

		assert.NotEmpty(t, v.hostnameMetadata.Hostname)

		if tt.expectLegacyHostname && !tt.imdsEnabled {
			assert.NotEmpty(t, v.hostnameMetadata.LegacyResolutionHostname)
			assert.NotEqual(t, v.hostnameMetadata.Hostname, v.hostnameMetadata.LegacyResolutionHostname)
			assert.Equal(t, v.hostnameMetadata.LegacyResolutionHostname, osHostname)
		} else {
			assert.Empty(t, v.hostnameMetadata.LegacyResolutionHostname)
		}

		if tt.imdsEnabled {
			assert.Equal(t, v.hostnameMetadata.Hostname, instanceID)
		} else {
			assert.Equal(t, v.hostnameMetadata.Hostname, osHostname)
		}
	})
}

func (v *baseHostnameSuite) TestIMDSv1Enable() {
	tests := []struct {
		name                 string
		agentConfig          string
		imdsEnabled          bool
		expectLegacyHostname bool
	}{
		{"IMDSv2TransitionEnabled", `ec2_prefer_imdsv2: false
ec2_imdsv2_transition_payload_enabled: true`, true, true},
		{"IMDSv2Disabled", `ec2_prefer_imdsv2: false
ec2_imdsv2_transition_payload_enabled: false`, true, false},
		{"IMDSv2Forced", `ec2_prefer_imdsv2: true
ec2_imdsv2_transition_payload_enabled: true`, true, false},
		{"IMDSv2Enable", `ec2_prefer_imdsv2: true
ec2_imdsv2_transition_payload_enabled: false`, true, false},
	}

	for _, tt := range tests {
		instanceOpts := []awshost.ProvisionerOption{
			awshost.WithAgentOptions(agentparams.WithAgentConfig(tt.agentConfig)),
		}
		runHostnameTest(v, instanceOpts, tt)
	}
}

func (v *baseHostnameSuite) TestIMDSv1Disable() {
	tests := []struct {
		name                 string
		agentConfig          string
		imdsEnabled          bool
		expectLegacyHostname bool
	}{
		{"IMDSv2TransitionEnabled", `ec2_prefer_imdsv2: false
ec2_imdsv2_transition_payload_enabled: true`, true, true},
		{"IMDSv2Disabled", `ec2_prefer_imdsv2: false
ec2_imdsv2_transition_payload_enabled: false`, false, false},
		{"IMDSv2Forced", `ec2_prefer_imdsv2: true
ec2_imdsv2_transition_payload_enabled: true`, true, false},
		{"IMDSv2Enable", `ec2_prefer_imdsv2: true
ec2_imdsv2_transition_payload_enabled: false`, true, false},
	}

	for _, tt := range tests {
		instanceOpts := []awshost.ProvisionerOption{
			awshost.WithAgentOptions(agentparams.WithAgentConfig(tt.agentConfig)),
			awshost.WithEC2InstanceOptions(ec2.WithIMDSv1Disabled()),
		}
		runHostnameTest(v, instanceOpts, tt)
	}
}
