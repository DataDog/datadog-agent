// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname contains helpers and e2e tests of the hostname resolution
package hostname

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

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
	Hostname                 string `json:"hostname"`
	LegacyResolutionHostname string `json:"legacy-resolution-hostname,omitempty"`
}

type baseHostnameSuite struct {
	e2e.BaseSuite[environments.Host]

	hostnameMetadata Meta
}

func TestBaseHostnameSuite(t *testing.T) {
	e2e.Run(t, &baseHostnameSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func requestAgentHostnameMetadataPayload(v *baseHostnameSuite) Meta {
	v.T().Helper()

	// Get the hostname and legacy-resolution-hostname from the agent status
	statusArgs := agentclient.WithArgs([]string{"header", "--json"})
	statusPayload := v.Env().Agent.Client.Status(statusArgs)

	type metadata struct {
		MetaPayload Meta `json:"meta"`
	}

	type status struct {
		Data metadata `json:"metadata"`
	}

	var result status
	if err := json.Unmarshal([]byte(statusPayload.Content), &result); err != nil {
		v.T().Fatal(err)
	}

	return result.Data.MetaPayload
}

// retrieveInstanceHostname retrieves the OS hostname from the EC2 metadata
func retrieveInstanceHostname(ec2Client *client.EC2Metadata) string {
	return strings.Split(ec2Client.Get("hostname"), ".")[0]
}

// retrieveInstanceID retrieves the instance ID from the EC2 metadata
func retrieveInstanceID(ec2Client *client.EC2Metadata) string {
	return ec2Client.Get("instance-id")
}

// runHostnameTest runs the hostname test with the given parameters
func runHostnameTest(v *baseHostnameSuite, instanceOpts []awshost.ProvisionerOption, tt struct {
	name                             string
	ec2PreferIMDSv2                  bool
	legacyResolutionHostname         bool
	expectedHostname                 func(*client.EC2Metadata) string
	expectedLegacyResolutionHostname func(*client.EC2Metadata) string
}) {
	v.T().Run(tt.name, func(t *testing.T) {
		v.UpdateEnv(awshost.ProvisionerNoFakeIntake(instanceOpts...))
		ec2Client := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)

		v.hostnameMetadata = requestAgentHostnameMetadataPayload(v)
		assert.NotEmpty(t, v.hostnameMetadata.Hostname)

		assert.Equal(t, tt.expectedHostname(ec2Client), v.hostnameMetadata.Hostname)
		// legacy-resolution-hostname should only be set when IMDSv1 is disabled, ec2_prefer_imdsv2 is disabled and legacy-resolution-hostname is enabled
		if tt.expectedLegacyResolutionHostname != nil {
			assert.NotEmpty(t, v.hostnameMetadata.LegacyResolutionHostname)
			assert.NotEqual(t, v.hostnameMetadata.Hostname, v.hostnameMetadata.LegacyResolutionHostname)
			assert.Equal(t, tt.expectedLegacyResolutionHostname(ec2Client), v.hostnameMetadata.LegacyResolutionHostname)
		} else {
			assert.Empty(t, v.hostnameMetadata.LegacyResolutionHostname)
		}
	})
}

// TestWithIMDSv1 tests the default hostname resolution for ec2 instances when IMDSv1 is enabled which means the hostname should always be the instance ID
func (v *baseHostnameSuite) TestWithIMDSv1() {
	tests := []struct {
		name                             string
		ec2PreferIMDSv2                  bool
		legacyResolutionHostname         bool
		expectedHostname                 func(*client.EC2Metadata) string
		expectedLegacyResolutionHostname func(*client.EC2Metadata) string
	}{
		// IMDSv1 is enable, transition to IMDSv2 should not change any behavior, legacy-resolution-hostname should be ignored and not set
		{
			name:                             "WithIMDSv1_IMDSv2Transition_NoLegacy",
			ec2PreferIMDSv2:                  false,
			legacyResolutionHostname:         true,
			expectedHostname:                 retrieveInstanceID,
			expectedLegacyResolutionHostname: nil,
		},
		// IMDSv1 is enabled, IMDSv2 disabled, the hostname should be the instance ID
		{
			name:                             "WithIMDSv1_IMDSv2Disabled_InstanceIDHostname",
			ec2PreferIMDSv2:                  false,
			legacyResolutionHostname:         false,
			expectedHostname:                 retrieveInstanceID,
			expectedLegacyResolutionHostname: nil,
		},
		// IMDSv2 if forced, the hostname should be the instance ID and legacy-resolution-hostname should be ignored and not set
		{
			name:                             "WithIMDSv1_IMDSv2Forced_NoLegacy",
			ec2PreferIMDSv2:                  true,
			legacyResolutionHostname:         true,
			expectedHostname:                 retrieveInstanceID,
			expectedLegacyResolutionHostname: nil,
		},
		// IMDSv2 is forced, the hostname should be the instance ID
		{
			name:                             "WithIMDSv1_IMDSv2Enabled_InstanceIDHostname",
			ec2PreferIMDSv2:                  true,
			legacyResolutionHostname:         false,
			expectedHostname:                 retrieveInstanceID,
			expectedLegacyResolutionHostname: nil,
		},
	}

	for _, tt := range tests {
		agentConfig := fmt.Sprintf(`ec2_prefer_imdsv2: %t
ec2_imdsv2_transition_payload_enabled: %t`, tt.ec2PreferIMDSv2, tt.legacyResolutionHostname)

		instanceOpts := []awshost.ProvisionerOption{
			awshost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)),
		}
		runHostnameTest(v, instanceOpts, tt)
	}
}

// TestWithoutIMDSv1 tests the default hostname resolution for ec2 instances when IMDSv1 is disabled which means the hostname should be the OS hostname when IMDS (v1 or v2) is disabled
func (v *baseHostnameSuite) TestWithoutIMDSv1() {
	tests := []struct {
		name                             string
		ec2PreferIMDSv2                  bool
		legacyResolutionHostname         bool
		expectedHostname                 func(*client.EC2Metadata) string
		expectedLegacyResolutionHostname func(*client.EC2Metadata) string
	}{
		// IMDSv1 is disabled, IMDSv2 transition enabled, the hostname should be the instance ID and legacy-resolution-hostname should be set to the OS hostname
		{
			name:                             "WithoutIMDSv1_IMDSv2Transition_OSLegacyHostname",
			ec2PreferIMDSv2:                  false,
			legacyResolutionHostname:         true,
			expectedHostname:                 retrieveInstanceID,
			expectedLegacyResolutionHostname: retrieveInstanceHostname,
		},
		// IMDSv1 and IMDSv2 are disabled, the hostname should be the OS hostname
		{
			name:                             "WithoutIMDSv1_IMDSv2Disabled_OSHostname",
			ec2PreferIMDSv2:                  false,
			legacyResolutionHostname:         false,
			expectedHostname:                 retrieveInstanceHostname,
			expectedLegacyResolutionHostname: nil,
		},
		// IMDSv2 is forced, the hostname should be the instance ID and legacy-resolution-hostname should be ignored and not set
		{
			name:                             "WithoutIMDSv1_IMDSv2Forced_NoLegacy",
			ec2PreferIMDSv2:                  true,
			legacyResolutionHostname:         true,
			expectedHostname:                 retrieveInstanceID,
			expectedLegacyResolutionHostname: nil,
		},
		// IMDSv2 is forced, the hostname should be the instance ID
		{
			name:                             "WithoutIMDSv1_IMDSv2Enabled_InstanceIDHostname",
			ec2PreferIMDSv2:                  true,
			legacyResolutionHostname:         false,
			expectedHostname:                 retrieveInstanceID,
			expectedLegacyResolutionHostname: nil,
		},
	}

	for _, tt := range tests {
		agentConfig := fmt.Sprintf(`ec2_prefer_imdsv2: %t
ec2_imdsv2_transition_payload_enabled: %t`, tt.ec2PreferIMDSv2, tt.legacyResolutionHostname)

		instanceOpts := []awshost.ProvisionerOption{
			awshost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)),
			awshost.WithEC2InstanceOptions(ec2.WithIMDSv1Disable()),
		}
		runHostnameTest(v, instanceOpts, tt)
	}
}
