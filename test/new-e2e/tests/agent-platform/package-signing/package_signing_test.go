// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package packagesigningtest

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"

	"testing"

	"github.com/stretchr/testify/assert"
)

type Payload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  *signingMetadata `json:"signing_metadata"`
}

type signingMetadata struct {
	SigningKeys []SigningKey `json:"signing_keys"`
}

type SigningKey struct {
	Fingerprint    string         `json:"signing_key_fingerprint"`
	ExpirationDate string         `json:"signing_key_expiration_date"`
	KeyType        string         `json:"signing_key_type"`
	Repositories   []repositories `json:"repositories"`
}

type repositories struct {
	RepoName string `json:"repo_name"`
}

type packageSigningTestSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestPackageSigningComponent(t *testing.T) {
	t.Run(fmt.Sprintln("Test package signing on default ubuntu"), func(tt *testing.T) {
		tt.Parallel()
		fmt.Println("Testing on ubuntu")
		e2e.Run(tt,
			&packageSigningTestSuite{},
			e2e.FakeIntakeStackDefWithDefaultVMAndAgentClient(),
			params.WithStackName(fmt.Sprintln("pkgSigning-ubuntu", os.Getenv("CI_PIPELINE_ID"))),
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

}
