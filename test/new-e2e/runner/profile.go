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

type CloudProvider string

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

func newProfile(projectName string, environments []string, secretStore *parameters.Store) baseProfile {
	p := baseProfile{
		projectName:  projectName,
		environments: environments,
		store:        parameters.NewEnvStore(EnvPrefix),
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

func GetProfile() Profile {
	initProfile.Do(func() {
		var profileFunc func() (Profile, error) = NewLocalProfile
		isCI, _ := strconv.ParseBool(os.Getenv("CI"))
		if isCI || strings.ToLower(os.Getenv(strings.ToUpper(EnvPrefix+parameters.Profile))) == "ci" {
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
