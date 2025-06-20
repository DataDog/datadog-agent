// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package defaultscript contains default standard installation logic
package defaultscript

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	defaultInjectorVersion = "0.40.0-1"
)

var (
	// We use major version tagging
	defaultLibraryVersions = map[string]string{
		common.DatadogAPMLibraryJavaPackage:   "1",
		common.DatadogAPMLibraryRubyPackage:   "2",
		common.DatadogAPMLibraryJSPackage:     "5",
		common.DatadogAPMLibraryDotNetPackage: "3",
		common.DatadogAPMLibraryPythonPackage: "3",
		common.DatadogAPMLibraryPHPPackage:    "1",
	}

	fullSemverRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+`)

	// unsupportedEnvVars are the environment variables that are not supported by the default script
	unsupportedEnvVars = []string{
		"DD_INSTALLER",
		"DD_AGENT_FLAVOR",
		"DD_UPGRADE",
		"DD_INSTALL_ONLY",
		"DD_FIPS_MODE",
	}

	// supportedEnvVars are the environment variables that are supported by the default script to be reported
	// in the span
	supportedEnvVars = []string{
		"DD_ENV",
		"DD_SITE",
		"DD_TAGS",
		"DD_HOST_TAGS",
		"DD_URL",
		"DD_REMOTE_UPDATES",
		"DD_FIPS_MODE",
		"DD_SYSTEM_PROBE_ENSURE_CONFIG",
		"DD_RUNTIME_SECURITY_CONFIG_ENABLED",
		"DD_COMPLIANCE_CONFIG_ENABLED",
		"DD_APM_INSTRUMENTATION_ENABLED",
		"DD_APM_LIBRARIES",
		"DD_NO_AGENT_INSTALL",
		"DD_INSTALLER_REGISTRY_URL",
		"DD_INSTALLER_REGISTRY_AUTH",
		"DD_HOSTNAME",
		"DD_PROXY_HTTP",
		"DD_PROXY_HTTPS",
		"DD_PROXY_NO_PROXY",
	}
)

// SetupDefaultScript sets up the default installation
func SetupDefaultScript(s *common.Setup) error {
	// Telemetry
	telemetrySupportedEnvVars(s, supportedEnvVars...)
	if err := exitOnUnsupportedEnvVars(unsupportedEnvVars...); err != nil {
		return err
	}

	// Installer management
	setConfigInstallerDaemon(s)
	setConfigInstallerRegistries(s)

	// Config management
	setConfigTags(s)
	setConfigSecurityProducts(s)

	if url, ok := os.LookupEnv("DD_URL"); ok {
		s.Config.DatadogYAML.DDURL = url
	}

	// Install agent package
	installAgentPackage(s)

	// Optionally setup SSI
	err := SetupAPMSSIScript(s)
	if err != nil {
		return fmt.Errorf("failed to setup APM SSI script: %w", err)
	}

	return nil
}

// setConfigSecurityProducts sets the configuration for the security products
func setConfigSecurityProducts(s *common.Setup) {
	runtimeSecurityConfigEnabled, runtimeSecurityConfigEnabledOk := os.LookupEnv("DD_RUNTIME_SECURITY_CONFIG_ENABLED")
	complianceConfigEnabled, complianceConfigEnabledOk := os.LookupEnv("DD_COMPLIANCE_CONFIG_ENABLED")
	if runtimeSecurityConfigEnabledOk || complianceConfigEnabledOk {
		s.Config.SecurityAgentYAML = &config.SecurityAgentConfig{}
		s.Config.SystemProbeYAML = &config.SystemProbeConfig{}
	}
	if complianceConfigEnabledOk && strings.ToLower(complianceConfigEnabled) != "false" {
		s.Config.SecurityAgentYAML.ComplianceConfig = config.SecurityAgentComplianceConfig{
			Enabled: true,
		}
	}
	if runtimeSecurityConfigEnabledOk && strings.ToLower(runtimeSecurityConfigEnabled) != "false" {
		s.Config.SecurityAgentYAML.RuntimeSecurityConfig = config.RuntimeSecurityConfig{
			Enabled: true,
		}
		s.Config.SystemProbeYAML.RuntimeSecurityConfig = config.RuntimeSecurityConfig{
			Enabled: true,
		}
	}
}

// setConfigInstallerDaemon sets the daemon in the configuration
func setConfigInstallerDaemon(s *common.Setup) {
	s.Config.DatadogYAML.RemoteUpdates = true
	if val, ok := os.LookupEnv("DD_REMOTE_UPDATES"); ok && strings.ToLower(val) == "false" {
		s.Config.DatadogYAML.RemoteUpdates = false
	}
}

// setConfigInstallerRegistries sets the registries in the configuration
func setConfigInstallerRegistries(s *common.Setup) {
	registryURL, registryURLOk := os.LookupEnv("DD_INSTALLER_REGISTRY_URL")
	registryAuth, registryAuthOk := os.LookupEnv("DD_INSTALLER_REGISTRY_AUTH")
	if registryURLOk || registryAuthOk {
		s.Config.DatadogYAML.Installer = config.DatadogConfigInstaller{
			Registry: config.DatadogConfigInstallerRegistry{
				URL:  registryURL,
				Auth: registryAuth,
			},
		}
	}
}

// setConfigTags sets the tags in the configuration
func setConfigTags(s *common.Setup) {
	if tags, ok := os.LookupEnv("DD_TAGS"); ok {
		s.Config.DatadogYAML.Tags = strings.Split(tags, ",")
	} else {
		if tags, ok := os.LookupEnv("DD_HOST_TAGS"); ok {
			s.Config.DatadogYAML.Tags = strings.Split(tags, ",")
		}
	}
	if tags, ok := os.LookupEnv("DD_EXTRA_TAGS"); ok {
		s.Config.DatadogYAML.ExtraTags = strings.Split(tags, ",")
	}
}

// installAgentPackage installs the agent package
func installAgentPackage(s *common.Setup) {
	// Agent install
	if _, ok := os.LookupEnv("DD_NO_AGENT_INSTALL"); !ok {
		s.Packages.Install(common.DatadogAgentPackage, agentVersion())
	}
}

// installAPMPackages installs the APM packages
func installAPMPackages(s *common.Setup) {
	// Injector install
	_, apmInstrumentationEnabled := os.LookupEnv("DD_APM_INSTRUMENTATION_ENABLED")
	if apmInstrumentationEnabled {
		s.Packages.Install(common.DatadogAPMInjectPackage, defaultInjectorVersion)
	}

	// Libraries install
	_, installAllAPMLibraries := s.Env.ApmLibraries["all"]
	for _, library := range common.ApmLibraries {
		lang := packageToLanguage(library)
		_, installLibrary := s.Env.ApmLibraries[lang]
		if (installAllAPMLibraries || len(s.Env.ApmLibraries) == 0 && apmInstrumentationEnabled) || installLibrary {
			s.Packages.Install(library, getLibraryVersion(s.Env, library))
		}
	}
}

// packageToLanguage returns the language of an APM package
func packageToLanguage(packageName string) env.ApmLibLanguage {
	lang, found := strings.CutPrefix(packageName, "datadog-apm-library-")
	if !found {
		return ""
	}
	return env.ApmLibLanguage(lang)
}

// getLibraryVersion returns the version of the library to install
// It uses the version from the environment if available, otherwise it uses the default version.
// Maybe we should only use the default version?
func getLibraryVersion(env *env.Env, library string) string {
	version := "latest"
	if defaultVersion, ok := defaultLibraryVersions[library]; ok {
		version = defaultVersion
	}

	apmLibVersion := env.ApmLibraries[packageToLanguage(library)]
	if apmLibVersion == "" {
		return version
	}

	versionTag, _ := strings.CutPrefix(string(apmLibVersion), "v")
	if fullSemverRe.MatchString(versionTag) {
		return versionTag + "-1"
	}
	return versionTag
}

func exitOnUnsupportedEnvVars(envVars ...string) error {
	var unsupported []string
	for _, envVar := range envVars {
		if _, ok := os.LookupEnv(envVar); ok {
			unsupported = append(unsupported, envVar)
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported environment variables: %s, exiting setup", strings.Join(unsupported, ", "))
	}
	return nil
}

func telemetrySupportedEnvVars(s *common.Setup, envVars ...string) {
	for _, envVar := range envVars {
		s.Span.SetTag(fmt.Sprintf("env_var.%s", envVar), os.Getenv(envVar))
	}
}

func agentVersion() string {
	v := version.AgentVersion
	if !strings.HasSuffix(v, "-1") {
		v = v + "-1"
	}

	// Adapt version to OCI registry tags
	v = strings.ReplaceAll(v, "+", ".")
	v = strings.ReplaceAll(v, "~", "-")

	return v
}
