// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package packagesigningtest

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"testing"

	"github.com/stretchr/testify/require"
)

var (
	osVersion = flag.String("osversion", "", "platform/os version (debian-11)")
)

type packageSigningTestSuite struct {
	e2e.BaseSuite[environments.Host]
	osName string
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  *signingMetadata `json:"signing_metadata"`
}

type signingMetadata struct {
	SigningKeys []signingKey `json:"signing_keys"`
}

// signingKey represents relevant fields for a package signature key
type signingKey struct {
	Fingerprint    string       `json:"fingerprint"`
	ExpirationDate string       `json:"expiration_date"`
	KeyType        string       `json:"type"`
	Repositories   []Repository `json:"repositories"`
}

// Repository is a struct to store the repo name
type Repository struct {
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	GPGCheck     bool   `json:"gpgcheck"`
	RepoGPGCheck bool   `json:"repo_gpgcheck"`
}

func TestPackageSigningComponent(t *testing.T) {

	platformJSON := map[string]map[string]map[string]string{}
	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	nonAlpha := regexp.MustCompile("[^a-zA-Z]")
	platform := nonAlpha.ReplaceAllString(*osVersion, "")
	if platform == "sles" {
		platform = "suse"
	}
	architecture := "x86_64"
	if platformJSON[platform][architecture][*osVersion] == "" {
		// Fail if the image is not defined instead of silently running with default Ubuntu AMI
		t.Fatalf("No image found for %s %s %s", platform, architecture, *osVersion)
	}
	ami := platformJSON[platform][architecture][*osVersion]

	t.Run(fmt.Sprintf("Test package signing on %s\n", platform), func(tt *testing.T) {
		tt.Parallel()
		osDesc := platforms.BuildOSDescriptor(platform, architecture, *osVersion)
		e2e.Run(tt,
			&packageSigningTestSuite{osName: platform},
			e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
				awshost.WithEC2InstanceOptions(ec2.WithAMI(ami, osDesc, osDesc.Architecture)),
			)),
			//e2e.WithAgentParams(agentparams.WithPipeline(os.Getenv("CI_PIPELINE_ID"), "x86_64")), e2e.WithVMParams(ec2params.WithOS(testedOS))),
			e2e.WithStackName(fmt.Sprintf("pkgSigning-%s-%s", platform, os.Getenv("CI_PIPELINE_ID"))),
		)
	})
}

func (is *packageSigningTestSuite) TestPackageSigning() {
	diagnose := is.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "package-signing"}))
	t := is.T()
	t.Log(diagnose)
	var payload Payload
	err := json.Unmarshal([]byte(diagnose), &payload)
	if err != nil {
		t.Fatal(err)
	}
	assert.NotEmpty(t, payload.Metadata.SigningKeys)

	packageManager := "rpm"
	if is.osName == "ubuntu" || is.osName == "debian" {
		packageManager = "apt"
	}

	currentSigningKeys := map[string][]string{
		"apt": {"D75CEA17048B9ACBF186794B32637D44F14F620E", "C02432A9AEA46C8F5A1C68A5E7F854C410D33C42", "DBD145AB63EAC0BEE68F004D33EE313BAD9589B7", "5F1E256061D813B125E156E8E6266D4AC0962C7D", "A2923DFF56EDA6E76E55E492D3A80E30382E94DE"},
		"rpm": {"A4C0B90D7443CF6E4E8AA341F1068E14E09422B3", "C6559B690CA882F023BDF3F63F4D1729FD4BF915", "7408BFD56BC5BF0C361AAAE85D88EEA3B01082D3"},
	}
	var missingKeys []string
	for _, currentKey := range currentSigningKeys[packageManager] {
		missingKey := true
		for _, signingKey := range payload.Metadata.SigningKeys {
			if signingKey.Fingerprint == currentKey {
				missingKey = false
			}
		}
		if missingKey {
			missingKeys = append(missingKeys, currentKey)
		}
	}
	if len(missingKeys) > 0 {
		t.Errorf("Missing keys: %v", missingKeys)
	}

}
