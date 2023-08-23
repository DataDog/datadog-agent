// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

// CloudProvider alias to string
type CloudProvider string

const (
	// AWS cloud provider
	AWS CloudProvider = "aws"
	// Azure cloud provider
	Azure CloudProvider = "az"
	// GCP cloud provider
	GCP CloudProvider = "gcp"
	// EnvPrefix prefix for e2e environment variables
	EnvPrefix = "E2E_"

	envSep = ","
)

var (
	workspaceFolder = os.TempDir()
	runProfile      Profile
	initProfile     sync.Once
)

// Profile interface defines functions required by a profile
type Profile interface {
	// EnvironmentName returns the environment names for cloud providers
	EnvironmentNames() string
	// ProjectName used by Pulumi
	ProjectName() string
	// RootWorkspacePath returns the root directory for local Pulumi workspace
	RootWorkspacePath() string
	// ParamStore() returns the normal parameter store
	ParamStore() parameters.Store
	// SecretStore returns the secure parameter store
	SecretStore() parameters.Store
	// NamePrefix returns a prefix to name objects
	NamePrefix() string
	// AllowDevMode returns if DevMode is allowed
	AllowDevMode() bool
}

// Shared implementations for common profiles methods
type baseProfile struct {
	projectName  string
	environments []string
	store        parameters.Store
	secretStore  parameters.Store
}

func newProfile(projectName string, environments []string, store parameters.Store, secretStore *parameters.Store) baseProfile {
	p := baseProfile{
		projectName:  projectName,
		environments: environments,
		store:        store,
	}

	if secretStore == nil {
		p.secretStore = p.store
	} else {
		p.secretStore = *secretStore
	}

	return p
}

// EnvironmentNames returns a comma-separated list of environments that the profile targets
func (p baseProfile) EnvironmentNames() string {
	return strings.Join(p.environments, envSep)
}

// ProjectName returns the project name
func (p baseProfile) ProjectName() string {
	return p.projectName
}

// ParamStore returns the parameters store
func (p baseProfile) ParamStore() parameters.Store {
	return p.store
}

// SecretStore returns the secret parameters store
func (p baseProfile) SecretStore() parameters.Store {
	return p.secretStore
}

// GetProfile return a profile initialising it at first call
func GetProfile() Profile {
	initProfile.Do(func() {
		var profileFunc func() (Profile, error) = NewLocalProfile
		isCI, _ := strconv.ParseBool(os.Getenv("CI"))
		if isCI || strings.ToLower(os.Getenv(strings.ToUpper(EnvPrefix+string(parameters.Profile)))) == "ci" {
			profileFunc = NewCIProfile
		}

		var err error
		runProfile, err = profileFunc()
		if err != nil {
			panic(err)
		}
	})

	return runProfile
}
