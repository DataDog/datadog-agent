// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains tests for the datadog installer
package installer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"os"
	"strings"
)

// TestPackageConfig is a struct that regroups the fields necessary to install a package from an OCI Registry
type TestPackageConfig struct {
	// Name the name of the package
	Name string
	// Alias Sometimes the package is named differently in some registries
	Alias string
	// Version the version to install
	Version string
	// Registry the URL of the registry
	Registry string
	// Auth the authentication method, "" for no authentication
	Auth string
}

// PackageOption is an optional function parameter type for the Datadog Installer
type PackageOption func(*TestPackageConfig) error

// WithAuthentication uses a specific authentication for a Registry to install the package.
func WithAuthentication(auth string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Auth = auth
		return nil
	}
}

// WithRegistry uses a specific Registry from where to install the package.
func WithRegistry(registryURL string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Registry = registryURL
		return nil
	}
}

// WithVersion uses a specific version of the package.
func WithVersion(version string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Version = version
		return nil
	}
}

// WithAlias specifies the package's alias.
func WithAlias(alias string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Alias = alias
		return nil
	}
}

// PackagesConfig is the list of known packages configuration for testing
var PackagesConfig = []TestPackageConfig{
	{Name: "datadog-installer", Version: fmt.Sprintf("pipeline-%v", os.Getenv("E2E_PIPELINE_ID")), Registry: "669783387624.dkr.ecr.us-east-1.amazonaws.com", Auth: "ecr"},
	{Name: "datadog-agent", Alias: "agent-package", Version: fmt.Sprintf("pipeline-%v", os.Getenv("E2E_PIPELINE_ID")), Registry: "669783387624.dkr.ecr.us-east-1.amazonaws.com", Auth: "ecr"},
	{Name: "datadog-apm-inject", Version: "latest"},
	{Name: "datadog-apm-library-java", Version: "latest"},
	{Name: "datadog-apm-library-ruby", Version: "latest"},
	{Name: "datadog-apm-library-js", Version: "latest"},
	{Name: "datadog-apm-library-dotnet", Version: "latest"},
	{Name: "datadog-apm-library-python", Version: "latest"},
}

func installScriptPackageManagerEnv(env map[string]string, arch e2eos.Architecture) {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if apiKey == "" || err != nil {
			apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
		}
	}
	env["DD_API_KEY"] = apiKey
	env["DD_SITE"] = "datadoghq.com"
	// Install Script env variables
	env["DD_INSTALLER"] = "true"
	env["TESTING_KEYS_URL"] = "keys.datadoghq.com"
	env["TESTING_APT_URL"] = "apttesting.datad0g.com"
	env["TESTING_APT_REPO_VERSION"] = fmt.Sprintf("pipeline-%s-a7-%s 7", os.Getenv("E2E_PIPELINE_ID"), arch)
	env["TESTING_YUM_URL"] = "yumtesting.datad0g.com"
	env["TESTING_YUM_VERSION_PATH"] = fmt.Sprintf("testing/pipeline-%s-a7/7", os.Getenv("E2E_PIPELINE_ID"))
}

func installScriptInstallerEnv(env map[string]string, packagesConfig []TestPackageConfig) {
	for _, pkg := range packagesConfig {
		name := strings.ToUpper(strings.ReplaceAll(pkg.Name, "-", "_"))
		image := strings.TrimPrefix(name, "DATADOG_") + "_PACKAGE"
		if pkg.Registry != "" {
			env[fmt.Sprintf("DD_INSTALLER_REGISTRY_URL_%s", image)] = pkg.Registry
		}
		if pkg.Auth != "" {
			env[fmt.Sprintf("DD_INSTALLER_REGISTRY_AUTH_%s", image)] = pkg.Auth
		}
		if pkg.Version != "" && pkg.Version != "latest" {
			env[fmt.Sprintf("DD_INSTALLER_DEFAULT_PKG_VERSION_%s", name)] = pkg.Version
		}
	}
}

// InstallScriptEnv returns the environment variables for the install script
func InstallScriptEnv(arch e2eos.Architecture) map[string]string {
	return InstallScriptEnvWithPackages(arch, PackagesConfig)
}

// InstallScriptEnvWithPackages returns the environment variables for the install script for the given packages
func InstallScriptEnvWithPackages(arch e2eos.Architecture, packagesConfig []TestPackageConfig) map[string]string {
	env := map[string]string{}
	installScriptPackageManagerEnv(env, arch)
	installScriptInstallerEnv(env, packagesConfig)
	return env
}
