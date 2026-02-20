// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package packagesigningtest

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	"testing"
)

var (
	osDescriptors = flag.String("osdescriptors", "", "platform/arch/os version (debian/x86_64/11)")
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
	osDesc, err := platforms.BuildOSDescriptor(*osDescriptors)
	if err != nil {
		t.Fatalf("failed to build os descriptor: %v", err)
	}

	t.Run(fmt.Sprintf("Test package signing on %s\n", platforms.PrettifyOsDescriptor(osDesc)), func(tt *testing.T) {
		tt.Parallel()
		e2e.Run(tt,
			&packageSigningTestSuite{osName: osDesc.Flavor.String()},
			e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
				awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(osDesc))),
			)),
			e2e.WithStackName("pkgSigning-"+osDesc.Flavor.String()),
		)
	})
}

func (is *packageSigningTestSuite) TestPackageSigning() {
	// Install the signing keys
	if is.osName == "ubuntu" || is.osName == "debian" {
		is.Env().RemoteHost.MustExecute("sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl gnupg")
		aptUsrShareKeyring := "/usr/share/keyrings/datadog-archive-keyring.gpg"
		is.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo touch %s && sudo chmod a+r %s", aptUsrShareKeyring, aptUsrShareKeyring))
		keys := []string{"DATADOG_APT_KEY_CURRENT.public", "DATADOG_APT_KEY_C0962C7D.public", "DATADOG_APT_KEY_F14F620E.public", "DATADOG_APT_KEY_382E94DE.public"}
		for _, key := range keys {
			is.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo curl --retry 5 -o \"/tmp/%s\" \"https://keys.datadoghq.com/%s\"", key, key))
			is.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo cat \"/tmp/%s\" | sudo gpg --import --batch --no-default-keyring --keyring \"%s\"", key, aptUsrShareKeyring))
		}
		aptTrustedKeyring := "/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg"
		is.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo cp %s %s", aptUsrShareKeyring, aptTrustedKeyring))
	} else {
		keys := []string{"DATADOG_RPM_KEY_E09422B3.public", "DATADOG_RPM_KEY_CURRENT.public", "DATADOG_RPM_KEY_FD4BF915.public", "DATADOG_RPM_KEY_E09422B3.public"}
		for _, key := range keys {
			is.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo curl --retry 5 -o \"/tmp/%s\" \"https://keys.datadoghq.com/%s\"", key, key))
			is.Env().RemoteHost.MustExecute("sudo rpm --import /tmp/" + key)
		}
	}

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
