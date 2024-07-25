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

const (
	// StableVersion the latest stable version of the Datadog Installer
	StableVersion string = "7.56.0-installer-0.4.5"
)

var (
	// StableVersionPackage the latest stable version of the Datadog Installer in package format
	StableVersionPackage = fmt.Sprintf("%s-1", StableVersion)
)

type testPackageConfig struct {
	name           string
	defaultVersion string
	registry       string
	auth           string
}

var packagesConfig = []testPackageConfig{
	{name: "datadog-installer", defaultVersion: fmt.Sprintf("pipeline-%v", os.Getenv("E2E_PIPELINE_ID")), registry: "669783387624.dkr.ecr.us-east-1.amazonaws.com", auth: "ecr"},
	{name: "datadog-agent", defaultVersion: fmt.Sprintf("pipeline-%v", os.Getenv("E2E_PIPELINE_ID")), registry: "669783387624.dkr.ecr.us-east-1.amazonaws.com", auth: "ecr"},
	{name: "datadog-apm-inject", defaultVersion: "latest"},
	{name: "datadog-apm-library-java", defaultVersion: "latest"},
	{name: "datadog-apm-library-ruby", defaultVersion: "latest"},
	{name: "datadog-apm-library-js", defaultVersion: "latest"},
	{name: "datadog-apm-library-dotnet", defaultVersion: "latest"},
	{name: "datadog-apm-library-python", defaultVersion: "latest"},
}

func installScriptPackageManagerEnv(env map[string]string, arch e2eos.Architecture) {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
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

func installScriptInstallerEnv(env map[string]string) {
	for _, pkg := range packagesConfig {
		name := strings.ToUpper(strings.ReplaceAll(pkg.name, "-", "_"))
		image := strings.TrimPrefix(name, "DATADOG_") + "_PACKAGE"
		if pkg.registry != "" {
			env[fmt.Sprintf("DD_INSTALLER_REGISTRY_URL_%s", image)] = pkg.registry
		}
		if pkg.auth != "" {
			env[fmt.Sprintf("DD_INSTALLER_REGISTRY_AUTH_%s", image)] = pkg.auth
		}
		if pkg.defaultVersion != "" && pkg.defaultVersion != "latest" {
			env[fmt.Sprintf("DD_INSTALLER_DEFAULT_PKG_VERSION_%s", name)] = pkg.defaultVersion
		}
	}
}

// InstallScriptEnv returns the environment variables for the install script
func InstallScriptEnv(arch e2eos.Architecture) map[string]string {
	env := map[string]string{}
	installScriptPackageManagerEnv(env, arch)
	installScriptInstallerEnv(env)
	return env
}
