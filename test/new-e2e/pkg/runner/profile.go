// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"

	"testing"
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
	// ParamStore() returns the normal parameter store
	ParamStore() parameters.Store
	// SecretStore returns the secure parameter store
	SecretStore() parameters.Store
	// NamePrefix returns a prefix to name objects
	NamePrefix() string
	// AllowDevMode returns if DevMode is allowed
	AllowDevMode() bool
	// GetOutputDir returns the root output directory for tests to store output files and artifacts.
	// e.g. /tmp/e2e-output/2020-01-01_00-00-00_<random>
	//
	// See GetTestOutputDir for a function that returns a subdirectory for a specific test.
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
	outputRootFolder        string
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

// GetOutputDir returns the root output directory for tests to store output files and artifacts.
// The directory is created on the first call to this function, normally this will be when a
// test calls GetTestOutputDir.
//
// The root output directory is chosen in the following order:
//   - outputDir parameter from the runner configuration, or E2E_OUTPUT_DIR environment variable
//   - default provided by a parent profile, <defaultOutputRootFolder>/e2e-output, e.g. $CI_PROJECT_DIR/e2e-output
//   - os.TempDir()/e2e-output
//
// A timestamp is appended to the root output directory to distinguish between multiple runs,
// and os.MkdirTemp() is used to avoid name collisions between parallel runs.
//
// See GetTestOutputDir for a function that returns a subdirectory for a specific test.
func (p baseProfile) GetOutputDir() (string, error) {
	if p.outputRootFolder == "" {
		var outputRoot string
		configOutputRoot, err := p.store.GetWithDefault(parameters.OutputDir, "")
		if err != nil {
			return "", err
		}
		if configOutputRoot != "" {
			// If outputRoot is provided in the config file, use it as the root directory
			outputRoot = configOutputRoot
		} else if p.defaultOutputRootFolder != "" {
			// If a default outputRoot was provided, use it as the root directory
			outputRoot = filepath.Join(p.defaultOutputRootFolder, "e2e-output")
		} else if outputRoot == "" {
			// If outputRoot is not provided, use os.TempDir() as the root directory
			outputRoot = filepath.Join(os.TempDir(), "e2e-output")
		}
		// Append timestamp to distinguish between multiple runs
		// Format: YYYY-MM-DD_HH-MM-SS
		// Use a custom timestamp format because Windows paths can't contain ':' characters
		// and we don't need the timezone information.
		timePart := time.Now().Format("2006-01-02_15-04-05")
		// create root directory
		err = os.MkdirAll(outputRoot, 0755)
		if err != nil {
			return "", err
		}
		// Create final output directory
		// Use MkdirTemp to avoid name collisions between parallel runs
		outputRoot, err = os.MkdirTemp(outputRoot, fmt.Sprintf("%s_*", timePart))
		if err != nil {
			return "", err
		}
		p.outputRootFolder = outputRoot
	}
	return p.outputRootFolder, nil
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

// GetTestOutputDir returns the output directory for a specific test.
// The test name is sanitized to remove invalid characters, and the output directory is created.
func GetTestOutputDir(p Profile, t *testing.T) (string, error) {
	// https://en.wikipedia.org/wiki/Filename#Reserved_characters_and_words
	invalidPathChars := strings.Join([]string{"?", "%", "*", ":", "|", "\"", "<", ">", ".", ",", ";", "="}, "")
	root, err := p.GetOutputDir()
	if err != nil {
		return "", err
	}
	testPart := strings.ReplaceAll(t.Name(), invalidPathChars, "_")
	path := filepath.Join(root, testPart)
	err = os.MkdirAll(path, 0755)
	if err != nil {
		return "", err
	}
	return path, nil
}
