// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
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
	defaultWorkspaceRootFolder = path.Join(os.TempDir(), "pulumi-workspace")
	runProfile                 Profile
	initProfile                sync.Once
)

// Profile interface defines functions required by a profile
type Profile interface {
	// EnvironmentName returns the environment names for cloud providers
	EnvironmentNames() string
	// ProjectName used by Pulumi
	ProjectName() string
	// GetWorkspacePath returns the directory for local Pulumi workspace.
	// Since one Workspace supports one single program and we have one program per stack,
	// the path should be unique for each stack.
	GetWorkspacePath(stackName string) string
	// ParamStore returns the normal parameter store
	ParamStore() parameters.Store
	// SecretStore returns the secure parameter store
	SecretStore() parameters.Store
	// NamePrefix returns a prefix to name objects
	NamePrefix() string
	// AllowDevMode returns if DevMode is allowed
	AllowDevMode() bool
	// GetOutputDir returns the root output directory for tests to store output files and artifacts.
	// e.g. /tmp/e2e-output/ or ~/e2e-output/
	//
	// It is recommended to use GetTestOutputDir to create a subdirectory for a specific test.
	GetOutputDir() (string, error)
}

// Shared implementations for common profiles methods
type baseProfile struct {
	projectName             string
	environments            []string
	store                   parameters.Store
	secretStore             parameters.Store
	workspaceRootFolder     string
	defaultOutputRootFolder string
}

func newProfile(projectName string, environments []string, store parameters.Store, secretStore *parameters.Store, defaultOutputRoot string) baseProfile {
	p := baseProfile{
		projectName:             projectName,
		environments:            environments,
		store:                   store,
		workspaceRootFolder:     defaultWorkspaceRootFolder,
		defaultOutputRootFolder: defaultOutputRoot,
	}

	if secretStore == nil {
		p.secretStore = p.store
	} else {
		p.secretStore = *secretStore
	}

	return p
}

// mergeEnvironments returns a string with a space separated list of available environments. It merges environments with a `defaultEnvironments` map
func mergeEnvironments(environments string, defaultEnvironments map[string]string) string {
	environmentsList := strings.Split(environments, " ")
	// set merged map capacity to worst case scenario of no overlapping key
	mergedEnvironmentsMap := make(map[string]string, len(defaultEnvironments)+len(environmentsList))
	maps.Copy(mergedEnvironmentsMap, defaultEnvironments)
	for _, env := range environmentsList {
		parts := strings.Split(env, "/")
		if len(parts) == 2 {
			mergedEnvironmentsMap[parts[0]] = parts[1]
		}
	}

	mergedEnvironmentsList := make([]string, 0, len(mergedEnvironmentsMap))
	for k, v := range mergedEnvironmentsMap {
		mergedEnvironmentsList = append(mergedEnvironmentsList, fmt.Sprintf("%s/%s", k, v))
	}

	return strings.Join(mergedEnvironmentsList, " ")
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

// GetOutputDir returns the root output directory to be used to store output files and artifacts.
// A path is returned but the directory is not created.
//
// The root output directory is chosen in the following order:
//   - outputDir parameter from the runner configuration, or E2E_OUTPUT_DIR environment variable
//   - default provided by profile, <defaultOutputRootFolder>/e2e-output, e.g. $CI_PROJECT_DIR/e2e-output
//   - os.TempDir()/e2e-output
//
// See GetTestOutputDir for a function that returns a subdirectory for a specific test.
func (p baseProfile) GetOutputDir() (string, error) {
	configOutputRoot, err := p.store.GetWithDefault(parameters.OutputDir, "")
	if err != nil {
		return "", err
	}
	if configOutputRoot != "" {
		// If outputRoot is provided in the config file, use it as the root directory
		return configOutputRoot, nil
	}
	if p.defaultOutputRootFolder != "" {
		// If a default outputRoot was provided, use it as the root directory
		return filepath.Join(p.defaultOutputRootFolder, "e2e-output"), nil
	}
	// as a final fallback, use os.TempDir() as the root directory
	return filepath.Join(os.TempDir(), "e2e-output"), nil
}

// GetWorkspacePath returns the directory for CI Pulumi workspace.
// Since one Workspace supports one single program and we have one program per stack,
// the path should be unique for each stack.
func (p baseProfile) GetWorkspacePath(stackName string) string {
	return path.Join(p.workspaceRootFolder, hashString(stackName))
}

func hashString(s string) string {
	hasher := fnv.New64a()
	_, _ = io.WriteString(hasher, s)
	return fmt.Sprintf("%016x", hasher.Sum64())
}

// GetProfile return a profile initialising it at first call
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
