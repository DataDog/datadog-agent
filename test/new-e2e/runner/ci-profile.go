// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
)

type ciProfile struct {
	baseProfile

	ciUniqueID string
}

func NewCIProfile() (Profile, error) {
	// Create workspace directory
	if err := os.MkdirAll(workspaceFolder, 0o700); err != nil {
		return nil, fmt.Errorf("unable to create temporary folder at: %s, err: %w", workspaceFolder, err)
	}
	ciSecretPrefix := os.Getenv("CI_SECRET_PREFIX")
	if len(ciSecretPrefix) == 0 {
		ciSecretPrefix = DefaultCISecretPrefix
	}
	// Secret store
	secretStore := parameters.NewAWSStore(ciSecretPrefix)

	// Set Pulumi password
	passVal, err := secretStore.Get(parameters.PulumiPassword)
	if err != nil {
		return nil, fmt.Errorf("unable to get pulumi state password, err: %w", err)
	}
	os.Setenv("PULUMI_CONFIG_PASSPHRASE", passVal)

	// Building name prefix
	pipelineID := os.Getenv("CI_PIPELINE_ID")
	projectID := os.Getenv("CI_PROJECT_ID")
	if pipelineID == "" || projectID == "" {
		return nil, fmt.Errorf("unable to compute name prefix, missing variables pipeline id: %s, project id: %s", pipelineID, projectID)
	}

	ciEnvironments := strings.Split(os.Getenv("CI_ENV_NAMES"), " ")
	if len(ciEnvironments) == 0 {
		ciEnvironments = defaultCIEnvs
	}

	return ciProfile{
		baseProfile: newProfile("e2eci", ciEnvironments, &secretStore),
		ciUniqueID:  pipelineID + "-" + projectID,
	}, nil
}

func (p ciProfile) RootWorkspacePath() string {
	return workspaceFolder
}

func (p ciProfile) NamePrefix() string {
	return p.ciUniqueID
}

func (p ciProfile) AllowDevMode() bool {
	return false
}
