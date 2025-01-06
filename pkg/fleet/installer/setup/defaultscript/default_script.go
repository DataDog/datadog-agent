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
	// TODO: support as many as possible
	unsupportedEnvVars = []string{
		"DD_INSTALLER",
		"DD_AGENT_FLAVOR",
		"DD_URL",
		"DD_HOST_TAGS",
		"DD_UPGRADE",
		"DD_APM_INSTRUMENTATION_NO_CONFIG_CHANGE",
		"DD_INSTALL_ONLY",
	}
)

// SetupDefaultScript sets up the default installation
func SetupDefaultScript(s *common.Setup) error {
	// Installer management
	s.Config.DatadogYAML.RemoteUpdates = true
	s.Config.DatadogYAML.RemotePolicies = true
	if val, ok := os.LookupEnv("DD_REMOTE_UPDATES"); ok && strings.ToLower(val) == "false" {
		s.Config.DatadogYAML.RemoteUpdates = false
	}
	if val, ok := os.LookupEnv("DD_REMOTE_POLICIES"); ok && strings.ToLower(val) == "false" {
		s.Config.DatadogYAML.RemotePolicies = false
	}
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

	// Config management
	warnUnsupportedEnvVars(s, unsupportedEnvVars...)
	if tags, ok := os.LookupEnv("DD_TAGS"); ok {
		s.Config.DatadogYAML.Tags = strings.Split(tags, ",")
	}

	if _, ok := os.LookupEnv("DD_FIPS_MODE"); ok {
		return fmt.Errorf("the Datadog Installer doesn't support FIPS mode")
	}

	if url, ok := os.LookupEnv("DD_URL"); ok {
		s.Config.DatadogYAML.DDURL = url
	}

	// Feature enablement through environment variables
	_, systemProbeEnsureConfigOk := os.LookupEnv("DD_SYSTEM_PROBE_ENSURE_CONFIG")
	runtimeSecurityConfigEnabled, runtimeSecurityConfigEnabledOk := os.LookupEnv("DD_RUNTIME_SECURITY_CONFIG_ENABLED")
	complianceConfigEnabled, complianceConfigEnabledOk := os.LookupEnv("DD_COMPLIANCE_CONFIG_ENABLED")
	if systemProbeEnsureConfigOk || runtimeSecurityConfigEnabledOk || complianceConfigEnabledOk {
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

	// Agent install
	if _, ok := os.LookupEnv("DD_NO_AGENT_INSTALL"); !ok {
		s.Packages.Install(common.DatadogAgentPackage, agentVersion(s.Env))
	}

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

	return nil
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

func warnUnsupportedEnvVars(s *common.Setup, envVars ...string) {
	var setUnsupported []string
	for _, envVar := range envVars {
		if _, ok := os.LookupEnv(envVar); ok {
			setUnsupported = append(setUnsupported, envVar)
		}
	}
	s.Out.WriteString(fmt.Sprintf("Warning: options '%s' are not supported and will be ignored\n", strings.Join(setUnsupported, "', '")))
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
