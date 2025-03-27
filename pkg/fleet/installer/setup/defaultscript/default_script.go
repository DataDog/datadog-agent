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
)

const (
	defaultAgentVersion    = "7.60.1-1"
	defaultInjectorVersion = "0.26.0-1"
)

var (
	defaultLibraryVersions = map[string]string{
		common.DatadogAPMLibraryJavaPackage:   "1.44.1-1",
		common.DatadogAPMLibraryRubyPackage:   "2.8.0-1",
		common.DatadogAPMLibraryJSPackage:     "5.30.0-1",
		common.DatadogAPMLibraryDotNetPackage: "3.7.0-1",
		common.DatadogAPMLibraryPythonPackage: "2.9.2-1",
		common.DatadogAPMLibraryPHPPackage:    "1.5.1-1",
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

	// Install packages
	installAgentPackage(s)
	installAPMPackages(s)

	return nil
}

// setConfigSecurityProducts sets the configuration for the security products
func setConfigSecurityProducts(s *common.Setup) {
	runtimeSecurityConfigEnabled, runtimeSecurityConfigEnabledOk := os.LookupEnv("DD_RUNTIME_SECURITY_CONFIG_ENABLED")
	complianceConfigEnabled, complianceConfigEnabledOk := os.LookupEnv("DD_COMPLIANCE_CONFIG_ENABLED")
	if runtimeSecurityConfigEnabledOk || complianceConfigEnabledOk {
		s.Config.SecurityAgentYAML = &common.SecurityAgentConfig{}
		s.Config.SystemProbeYAML = &common.SystemProbeConfig{}
	}
	if complianceConfigEnabledOk && strings.ToLower(complianceConfigEnabled) != "false" {
		s.Config.SecurityAgentYAML.ComplianceConfig = common.SecurityAgentComplianceConfig{
			Enabled: true,
		}
	}
	if runtimeSecurityConfigEnabledOk && strings.ToLower(runtimeSecurityConfigEnabled) != "false" {
		s.Config.SecurityAgentYAML.RuntimeSecurityConfig = common.RuntimeSecurityConfig{
			Enabled: true,
		}
		s.Config.SystemProbeYAML.RuntimeSecurityConfig = common.RuntimeSecurityConfig{
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
		s.Config.DatadogYAML.Installer = common.DatadogConfigInstaller{
			Registry: common.DatadogConfigInstallerRegistry{
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
}

// installAgentPackage installs the agent package
func installAgentPackage(s *common.Setup) {
	// Agent install
	if _, ok := os.LookupEnv("DD_NO_AGENT_INSTALL"); !ok {
		s.Packages.Install(common.DatadogAgentPackage, agentVersion(s.Env))
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
		if (installAllAPMLibraries || len(s.Env.ApmLibraries) == 0 && apmInstrumentationEnabled) && library != common.DatadogAPMLibraryPHPPackage || installLibrary {
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

func agentVersion(e *env.Env) string {
	minorVersion := e.AgentMinorVersion
	if strings.Contains(minorVersion, ".") && !strings.HasSuffix(minorVersion, "-1") {
		minorVersion = minorVersion + "-1"
	}
	if minorVersion != "" {
		return "7." + minorVersion
	}
	return defaultAgentVersion
}
