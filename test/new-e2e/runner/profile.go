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

	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
)

// CloudProvider exported type should have comment or be unexported
type CloudProvider string

// This const block should have a comment or be unexported
const (
	AWS       CloudProvider = "aws"
	Azure     CloudProvider = "az"
	GCP       CloudProvider = "gcp"
	EnvPrefix               = "E2E_"

	envSep = ","
)

var (
	workspaceFolder = os.TempDir()
	runProfile      Profile
	initProfile     sync.Once
)

// Profile exported type should have comment or be unexported
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

func (p baseProfile) EnvironmentNames() string {
	return strings.Join(p.environments, envSep)
}

func (p baseProfile) ProjectName() string {
	return p.projectName
}

func (p baseProfile) ParamStore() parameters.Store {
	return p.store
}

func (p baseProfile) SecretStore() parameters.Store {
	return p.secretStore
}

// GetProfile exported function should have comment or be unexported
func GetProfile() Profile {
	initProfile.Do(func() {
		var profileFunc = NewLocalProfile
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
