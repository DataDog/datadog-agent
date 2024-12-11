// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

const (
	defaultCISecretPrefix = "ci.datadog-agent."
)

var defaultCIEnvironments = map[string]string{
	"aws": "agent-qa",
	"az":  "agent-qa",
	"gcp": "agent-qa",
}

type ciProfile struct {
	baseProfile

	ciUniqueID string
}

// NewCIProfile creates a new CI profile
func NewCIProfile() (Profile, error) {
	ciSecretPrefix := os.Getenv("CI_SECRET_PREFIX")
	if len(ciSecretPrefix) == 0 {
		ciSecretPrefix = defaultCISecretPrefix
	}
	// Secret store
	secretStore := parameters.NewAWSStore(ciSecretPrefix)

	// Set Pulumi password
	passVal, err := secretStore.Get(parameters.PulumiPassword)
	if err != nil {
		return nil, fmt.Errorf("unable to get pulumi state password, err: %w", err)
	}
	// TODO move to job script
	os.Setenv("PULUMI_CONFIG_PASSPHRASE", passVal)

	// Building name prefix
	jobID := os.Getenv("CI_JOB_ID")
	projectID := os.Getenv("CI_PROJECT_ID")
	if jobID == "" || projectID == "" {
		return nil, fmt.Errorf("unable to compute name prefix, missing variables job id: %s, project id: %s", jobID, projectID)
	}
	uniqueID := jobID
	store := parameters.NewEnvStore(EnvPrefix)

	initOnly, err := store.GetBoolWithDefault(parameters.InitOnly, false)
	if err != nil {
		return nil, err
	}

	preInitialized, err := store.GetBoolWithDefault(parameters.PreInitialized, false)
	if err != nil {
		return nil, err
	}

	if initOnly || preInitialized {
		uniqueID = fmt.Sprintf("init-%s", os.Getenv("CI_PIPELINE_ID")) // We use pipeline ID for init only and pre-initialized jobs, to be able to share state
	}

	// get environments from store
	environmentsStr, err := store.GetWithDefault(parameters.Environments, "")
	if err != nil {
		return nil, err
	}
	environmentsStr = mergeEnvironments(environmentsStr, defaultCIEnvironments)

	// TODO can be removed using E2E_ENV variable
	ciEnvNames := os.Getenv("CI_ENV_NAMES")
	if len(ciEnvNames) > 0 {
		environmentsStr = ciEnvNames
	}

	ciEnvironments := strings.Split(environmentsStr, " ")

	// get output root from env, if not found, empty string "" will tell base profile to use default
	outputRoot := os.Getenv("CI_PROJECT_DIR")

	return ciProfile{
		baseProfile: newProfile("e2eci", ciEnvironments, store, &secretStore, outputRoot),
		ciUniqueID:  "ci-" + uniqueID + "-" + projectID,
	}, nil
}

// NamePrefix returns a prefix to name objects based on a CI unique ID
func (p ciProfile) NamePrefix() string {
	return p.ciUniqueID
}

// AllowDevMode returns if DevMode is allowed
func (p ciProfile) AllowDevMode() bool {
	return false
}
