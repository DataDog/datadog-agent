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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"

	"testing"
)

var osVersion = flag.String("osversion", "", "os to test, eg debian or redhat")

type packageSigningTestSuite struct {
	e2e.Suite[e2e.AgentEnv]
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
	Fingerprint    string       `json:"signing_key_fingerprint"`
	ExpirationDate string       `json:"signing_key_expiration_date"`
	KeyType        string       `json:"signing_key_type"`
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

	e2eOSMapping := map[string]ec2os.Type{
		"debian":      ec2os.DebianOS,
		"ubuntu":      ec2os.UbuntuOS,
		"centos":      ec2os.CentOS,
		"amazonlinux": ec2os.AmazonLinuxOS,
		"redhat":      ec2os.RedHatOS,
		"rhel":        ec2os.RedHatOS,
		"sles":        ec2os.SuseOS,
		"fedora":      ec2os.FedoraOS,
		"suse":        ec2os.SuseOS,
		"rocky":       ec2os.RockyLinux,
	}

	var testedOS ec2os.Type
	nonAlpha := regexp.MustCompile("[^a-zA-Z]")
	rawOS := nonAlpha.ReplaceAllString(*osVersion, "")
	if value, ok := e2eOSMapping[rawOS]; ok {
		testedOS = value
	} else {
		t.Fatalf("OS %s not supported", *osVersion)
	}

	t.Run(fmt.Sprintf("Test package signing on %s\n", rawOS), func(tt *testing.T) {
		tt.Parallel()
		e2e.Run[e2e.AgentEnv](tt,
			&packageSigningTestSuite{osName: rawOS},
			e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithPipeline(os.Getenv("CI_PIPELINE_ID"), "x86_64")), e2e.WithVMParams(ec2params.WithOS(testedOS))),
			params.WithStackName(fmt.Sprintf("pkgSigning-%s-%s", rawOS, os.Getenv("CI_PIPELINE_ID"))),
		)
	})
}

func (is *packageSigningTestSuite) TestPackageSigning() {
	diagnose := is.Env().Agent.Diagnose(client.WithArgs([]string{"show-metadata", "package-signing"}))
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
