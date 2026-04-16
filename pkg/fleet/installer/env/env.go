// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package env provides the environment variables for the installer.
package env

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"golang.org/x/net/http/httpproxy"
)

const (
	envAPIKey                = "DD_API_KEY"
	envSite                  = "DD_SITE"
	envRemoteUpdates         = "DD_REMOTE_UPDATES"
	envOTelCollectorEnabled  = "DD_OTELCOLLECTOR_ENABLED"
	envMirror                = "DD_INSTALLER_MIRROR"
	envRegistryURL           = "DD_INSTALLER_REGISTRY_URL"
	envRegistryAuth          = "DD_INSTALLER_REGISTRY_AUTH"
	envRegistryUsername      = "DD_INSTALLER_REGISTRY_USERNAME"
	envRegistryPassword      = "DD_INSTALLER_REGISTRY_PASSWORD"
	envDefaultPackageVersion = "DD_INSTALLER_DEFAULT_PKG_VERSION"
	envDefaultPackageInstall = "DD_INSTALLER_DEFAULT_PKG_INSTALL"
	envApmLibraries          = "DD_APM_INSTRUMENTATION_LIBRARIES"
	envAgentMajorVersion     = "DD_AGENT_MAJOR_VERSION"
	envAgentMinorVersion     = "DD_AGENT_MINOR_VERSION"
	envApmLanguages          = "DD_APM_INSTRUMENTATION_LANGUAGES"
	envTags                  = "DD_TAGS"
	envExtraTags             = "DD_EXTRA_TAGS"
	envHostname              = "DD_HOSTNAME"
	envDDHTTPProxy           = "DD_PROXY_HTTP"
	envHTTPProxy             = "HTTP_PROXY"
	envDDHTTPSProxy          = "DD_PROXY_HTTPS"
	envHTTPSProxy            = "HTTPS_PROXY"
	envDDNoProxy             = "DD_PROXY_NO_PROXY"
	envNoProxy               = "NO_PROXY"
	envIsFromDaemon          = "DD_INSTALLER_FROM_DAEMON"
	envLogLevel              = "DD_LOG_LEVEL"

	// install script
	envApmInstrumentationEnabled   = "DD_APM_INSTRUMENTATION_ENABLED"
	envRuntimeMetricsEnabled       = "DD_RUNTIME_METRICS_ENABLED"
	envLogsInjection               = "DD_LOGS_INJECTION"
	envAPMTracingEnabled           = "DD_APM_TRACING_ENABLED"
	envProfilingEnabled            = "DD_PROFILING_ENABLED"
	envDataStreamsEnabled          = "DD_DATA_STREAMS_ENABLED"
	envAppsecEnabled               = "DD_APPSEC_ENABLED"
	envIastEnabled                 = "DD_IAST_ENABLED"
	envDataJobsEnabled             = "DD_DATA_JOBS_ENABLED"
	envAppsecScaEnabled            = "DD_APPSEC_SCA_ENABLED"
	envInfrastructureMode          = "DD_INFRASTRUCTURE_MODE"
	envAppKey                      = "DD_APP_KEY"
	envPAREnabled                  = "DD_PRIVATE_ACTION_RUNNER_ENABLED"
	envPARActionsAllowlist         = "DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST"
	envTracerLogsCollectionEnabled = "DD_APP_LOGS_COLLECTION_ENABLED"
	envRumEnabled                  = "DD_RUM_ENABLED"
	envRumApplicationID            = "DD_RUM_APPLICATION_ID"
	envRumClientToken              = "DD_RUM_CLIENT_TOKEN"
	envRumRemoteConfigurationID    = "DD_RUM_REMOTE_CONFIGURATION_ID"
	envRumSite                     = "DD_RUM_SITE"
)

// Windows MSI options
const (
	envAgentUserName = "DD_AGENT_USER_NAME"
	// envAgentUserNameCompat provides compatibility with the original MSI parameter name
	envAgentUserNameCompat = "DDAGENTUSER_NAME"
	envAgentUserPassword   = "DD_AGENT_USER_PASSWORD"
	// envAgentUserPasswordCompat provides compatibility with the original MSI parameter name
	envAgentUserPasswordCompat  = "DDAGENTUSER_PASSWORD"
	envProjectLocation          = "DD_PROJECTLOCATION"
	envApplicationDataDirectory = "DD_APPLICATIONDATADIRECTORY"
)

func newDefaultEnv() Env {
	return Env{
		Site: "datadoghq.com",

		RegistryOverrideByImage:     map[string]string{},
		RegistryAuthOverrideByImage: map[string]string{},
		RegistryUsernameByImage:     map[string]string{},
		RegistryPasswordByImage:     map[string]string{},

		DefaultPackagesInstallOverride: map[string]bool{},
		DefaultPackagesVersionOverride: map[string]string{},

		ApmLibraries: map[ApmLibLanguage]ApmLibVersion{},

		InstallScript: InstallScriptEnv{
			APMInstrumentationEnabled: APMInstrumentationNotSet,
		},

		Tags: []string{},

		ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
	}
}

// envBinding maps one environment variable to an Env field.
//
//   - fromOs reads an env var value into the Env struct. For prefix bindings,
//     value is "suffix=value" (one call per matching env var).
//   - toOs returns the value to emit for this env var. "" means skip. nil means read-only.
//   - prefixEnvVar: when true, Get() scans os.Environ() for envVar_SUFFIX=VALUE entries
//     and calls fromOs with "suffix=value" for each. ToEnv() uses toOsPrefix instead of toOs.
//   - toOsPrefix returns suffix→value for prefix bindings.
type envBinding struct {
	envVar       string
	prefixEnvVar bool
	fromOs       func(env *Env, value string)
	toOs         func(env *Env) string
	toOsPrefix   func(env *Env) map[string]string
}

var envBindings = []envBinding{
	// Core
	{envVar: envAPIKey,
		fromOs: func(e *Env, v string) {
			if v != "" {
				e.APIKey = v
			}
		},
		toOs: func(e *Env) string { return e.APIKey },
	},
	{envVar: envSite,
		fromOs: func(e *Env, v string) { e.Site = v },
		toOs:   func(e *Env) string { return e.Site },
	},
	{envVar: envRemoteUpdates,
		fromOs: func(e *Env, v string) { e.RemoteUpdates = strings.ToLower(v) == "true" },
		toOs: func(e *Env) string {
			if e.RemoteUpdates {
				return "true"
			}
			return ""
		},
	},
	{envVar: envOTelCollectorEnabled,
		fromOs: func(e *Env, v string) { e.OTelCollectorEnabled = strings.ToLower(v) == "true" },
		toOs: func(e *Env) string {
			if e.OTelCollectorEnabled {
				return "true"
			}
			return ""
		},
	},
	{envVar: envMirror,
		fromOs: func(e *Env, v string) { e.Mirror = v },
		toOs:   func(e *Env) string { return e.Mirror },
	},

	// Registry (single-var)
	{envVar: envRegistryURL,
		fromOs: func(e *Env, v string) { e.RegistryOverride = v },
		toOs:   func(e *Env) string { return e.RegistryOverride },
	},
	{envVar: envRegistryAuth,
		fromOs: func(e *Env, v string) { e.RegistryAuthOverride = v },
		toOs:   func(e *Env) string { return e.RegistryAuthOverride },
	},
	{envVar: envRegistryUsername,
		fromOs: func(e *Env, v string) { e.RegistryUsername = v },
		toOs:   func(e *Env) string { return e.RegistryUsername },
	},
	{envVar: envRegistryPassword,
		fromOs: func(e *Env, v string) { e.RegistryPassword = v },
		toOs:   func(e *Env) string { return e.RegistryPassword },
	},

	// Registry per-image overrides (prefix-scanning: DD_INSTALLER_REGISTRY_URL_<IMAGE>=<value>)
	{envVar: envRegistryURL, prefixEnvVar: true,
		fromOs:     func(e *Env, v string) { k, val, _ := strings.Cut(v, "="); e.RegistryOverrideByImage[k] = val },
		toOsPrefix: func(e *Env) map[string]string { return e.RegistryOverrideByImage },
	},
	{envVar: envRegistryAuth, prefixEnvVar: true,
		fromOs:     func(e *Env, v string) { k, val, _ := strings.Cut(v, "="); e.RegistryAuthOverrideByImage[k] = val },
		toOsPrefix: func(e *Env) map[string]string { return e.RegistryAuthOverrideByImage },
	},
	{envVar: envRegistryUsername, prefixEnvVar: true,
		fromOs:     func(e *Env, v string) { k, val, _ := strings.Cut(v, "="); e.RegistryUsernameByImage[k] = val },
		toOsPrefix: func(e *Env) map[string]string { return e.RegistryUsernameByImage },
	},
	{envVar: envRegistryPassword, prefixEnvVar: true,
		fromOs:     func(e *Env, v string) { k, val, _ := strings.Cut(v, "="); e.RegistryPasswordByImage[k] = val },
		toOsPrefix: func(e *Env) map[string]string { return e.RegistryPasswordByImage },
	},

	// Default package overrides (prefix-scanning)
	{envVar: envDefaultPackageInstall, prefixEnvVar: true,
		fromOs: func(e *Env, v string) {
			k, val, _ := strings.Cut(v, "=")
			e.DefaultPackagesInstallOverride[k] = strings.ToLower(val) == "true"
		},
		toOsPrefix: func(e *Env) map[string]string {
			m := make(map[string]string, len(e.DefaultPackagesInstallOverride))
			for k, v := range e.DefaultPackagesInstallOverride {
				if v {
					m[k] = "true"
				}
			}
			return m
		},
	},
	{envVar: envDefaultPackageVersion, prefixEnvVar: true,
		fromOs:     func(e *Env, v string) { k, val, _ := strings.Cut(v, "="); e.DefaultPackagesVersionOverride[k] = val },
		toOsPrefix: func(e *Env) map[string]string { return e.DefaultPackagesVersionOverride },
	},

	// APM libraries (DD_APM_INSTRUMENTATION_LANGUAGES is a lower-priority fallback)
	{envVar: envApmLanguages,
		fromOs: func(e *Env, v string) {
			if v != "" {
				e.ApmLibraries = parseAPMLanguagesValue(v)
			}
		},
	},
	{envVar: envApmLibraries,
		fromOs: func(e *Env, v string) { e.ApmLibraries = parseApmLibrariesValue(v) },
		toOs: func(e *Env) string {
			if len(e.ApmLibraries) == 0 {
				return ""
			}
			libraries := make([]string, 0, len(e.ApmLibraries))
			for l, v := range e.ApmLibraries {
				s := string(l)
				if v != "" {
					s += ":" + string(v)
				}
				libraries = append(libraries, s)
			}
			slices.Sort(libraries)
			return strings.Join(libraries, ",")
		},
	},

	// Agent version
	{envVar: envAgentMajorVersion, fromOs: func(e *Env, v string) { e.AgentMajorVersion = v }},
	{envVar: envAgentMinorVersion, fromOs: func(e *Env, v string) { e.AgentMinorVersion = v }},

	// Install script
	{envVar: envApmInstrumentationEnabled,
		fromOs: func(e *Env, v string) { e.InstallScript.APMInstrumentationEnabled = v },
		toOs:   func(e *Env) string { return e.InstallScript.APMInstrumentationEnabled },
	},
	{envVar: envRuntimeMetricsEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.RuntimeMetricsEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.RuntimeMetricsEnabled) },
	},
	{envVar: envLogsInjection,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.LogsInjection = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.LogsInjection) },
	},
	{envVar: envAPMTracingEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.APMTracingEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.APMTracingEnabled) },
	},
	{envVar: envProfilingEnabled,
		fromOs: func(e *Env, v string) { e.InstallScript.ProfilingEnabled = v },
		toOs:   func(e *Env) string { return e.InstallScript.ProfilingEnabled },
	},
	{envVar: envDataStreamsEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.DataStreamsEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.DataStreamsEnabled) },
	},
	{envVar: envAppsecEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.AppsecEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.AppsecEnabled) },
	},
	{envVar: envIastEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.IastEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.IastEnabled) },
	},
	{envVar: envDataJobsEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.DataJobsEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.DataJobsEnabled) },
	},
	{envVar: envAppsecScaEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.AppsecScaEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.AppsecScaEnabled) },
	},
	{envVar: envTracerLogsCollectionEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.TracerLogsCollectionEnabled = b
			}
		},
	},
	{envVar: envRumEnabled,
		fromOs: func(e *Env, v string) {
			if b := parseBoolPtr(v); b != nil {
				e.InstallScript.RumEnabled = b
			}
		},
		toOs: func(e *Env) string { return formatBoolPtr(e.InstallScript.RumEnabled) },
	},
	{envVar: envRumApplicationID,
		fromOs: func(e *Env, v string) { e.InstallScript.RumApplicationID = v },
		toOs:   func(e *Env) string { return e.InstallScript.RumApplicationID },
	},
	{envVar: envRumClientToken,
		fromOs: func(e *Env, v string) { e.InstallScript.RumClientToken = v },
		toOs:   func(e *Env) string { return e.InstallScript.RumClientToken },
	},
	{envVar: envRumRemoteConfigurationID,
		fromOs: func(e *Env, v string) { e.InstallScript.RumRemoteConfigurationID = v },
		toOs:   func(e *Env) string { return e.InstallScript.RumRemoteConfigurationID },
	},
	{envVar: envRumSite,
		fromOs: func(e *Env, v string) { e.InstallScript.RumSite = v },
		toOs:   func(e *Env) string { return e.InstallScript.RumSite },
	},

	// Tags
	{envVar: envTags,
		fromOs: func(e *Env, v string) {
			tags := strings.Split(v, ",")
			var result []string
			for _, t := range tags {
				trimmed := strings.TrimSpace(t)
				if trimmed != "" {
					result = append(result, trimmed)
				}
			}
			e.Tags = result
		},
		toOs: func(e *Env) string {
			if len(e.Tags) == 0 {
				return ""
			}
			return strings.Join(e.Tags, ",")
		},
	},
	{envVar: envExtraTags,
		fromOs: func(e *Env, v string) {
			tags := strings.Split(v, ",")
			var result []string
			for _, t := range tags {
				trimmed := strings.TrimSpace(t)
				if trimmed != "" {
					result = append(result, trimmed)
				}
			}
			e.Tags = append(e.Tags, result...)
		},
	},

	// Hostname
	{envVar: envHostname,
		fromOs: func(e *Env, v string) { e.Hostname = v },
		toOs:   func(e *Env) string { return e.Hostname },
	},

	// Proxy fallback chains (lowest priority first, highest last)
	{envVar: strings.ToLower(envHTTPProxy), fromOs: func(e *Env, v string) { e.HTTPProxy = v }},
	{envVar: envHTTPProxy,
		fromOs: func(e *Env, v string) { e.HTTPProxy = v },
		toOs:   func(e *Env) string { return e.HTTPProxy },
	},
	{envVar: envDDHTTPProxy, fromOs: func(e *Env, v string) { e.HTTPProxy = v }},

	{envVar: strings.ToLower(envHTTPSProxy), fromOs: func(e *Env, v string) { e.HTTPSProxy = v }},
	{envVar: envHTTPSProxy,
		fromOs: func(e *Env, v string) { e.HTTPSProxy = v },
		toOs:   func(e *Env) string { return e.HTTPSProxy },
	},
	{envVar: envDDHTTPSProxy, fromOs: func(e *Env, v string) { e.HTTPSProxy = v }},

	{envVar: strings.ToLower(envNoProxy), fromOs: func(e *Env, v string) { e.NoProxy = v }},
	{envVar: envNoProxy,
		fromOs: func(e *Env, v string) { e.NoProxy = v },
		toOs:   func(e *Env) string { return e.NoProxy },
	},
	{envVar: envDDNoProxy, fromOs: func(e *Env, v string) { e.NoProxy = v }},

	// Infrastructure
	{envVar: envInfrastructureMode,
		fromOs: func(e *Env, v string) { e.InfrastructureMode = v },
		toOs:   func(e *Env) string { return e.InfrastructureMode },
	},

	// PAR (AppKey and Allowlist are only emitted when PAREnabled)
	{envVar: envAppKey,
		fromOs: func(e *Env, v string) { e.AppKey = v },
		toOs: func(e *Env) string {
			if e.PAREnabled {
				return e.AppKey
			}
			return ""
		},
	},
	{envVar: envPAREnabled,
		fromOs: func(e *Env, v string) { e.PAREnabled = strings.ToLower(v) == "true" },
		toOs: func(e *Env) string {
			if e.PAREnabled {
				return "true"
			}
			return ""
		},
	},
	{envVar: envPARActionsAllowlist,
		fromOs: func(e *Env, v string) { e.PARActionsAllowlist = v },
		toOs: func(e *Env) string {
			if e.PAREnabled {
				return e.PARActionsAllowlist
			}
			return ""
		},
	},

	// Daemon flag
	{envVar: envIsFromDaemon,
		fromOs: func(e *Env, v string) { e.IsFromDaemon = strings.ToLower(v) == "true" },
		toOs: func(e *Env) string {
			if e.IsFromDaemon {
				return "true"
			}
			return ""
		},
	},

	// Log level (forced to "off" when running from the daemon)
	{envVar: envLogLevel,
		fromOs: func(e *Env, v string) { e.LogLevel = v },
		toOs: func(e *Env) string {
			if e.IsFromDaemon {
				return "off"
			}
			return ""
		},
	},

	// MSI compat vars (compat entries are read-only, primary entries emit)
	{envVar: envAgentUserNameCompat, fromOs: func(e *Env, v string) { e.MsiParams.AgentUserName = v }},
	{envVar: envAgentUserName,
		fromOs: func(e *Env, v string) { e.MsiParams.AgentUserName = v },
		toOs:   func(e *Env) string { return e.MsiParams.AgentUserName },
	},
	{envVar: envAgentUserPasswordCompat, fromOs: func(e *Env, v string) { e.MsiParams.AgentUserPassword = v }},
	{envVar: envAgentUserPassword,
		fromOs: func(e *Env, v string) { e.MsiParams.AgentUserPassword = v },
		toOs:   func(e *Env) string { return e.MsiParams.AgentUserPassword },
	},
	{envVar: envProjectLocation,
		fromOs: func(e *Env, v string) { e.MsiParams.ProjectLocation = v },
		toOs:   func(e *Env) string { return e.MsiParams.ProjectLocation },
	},
	{envVar: envApplicationDataDirectory,
		fromOs: func(e *Env, v string) { e.MsiParams.ApplicationDataDirectory = v },
		toOs:   func(e *Env) string { return e.MsiParams.ApplicationDataDirectory },
	},
}

// formatBoolPtr returns "true"/"false" for non-nil, "" for nil.
func formatBoolPtr(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

// parseBoolPtr parses "true"/"false" strings into a *bool.
// Returns nil for any other value (use for optional *bool fields).
func parseBoolPtr(value string) *bool {
	switch value {
	case "true":
		b := true
		return &b
	case "false":
		b := false
		return &b
	default:
		return nil
	}
}

// parseApmLibrariesValue parses the DD_APM_INSTRUMENTATION_LIBRARIES value.
func parseApmLibrariesValue(value string) map[ApmLibLanguage]ApmLibVersion {
	result := map[ApmLibLanguage]ApmLibVersion{}
	if value == "" {
		return result
	}
	for _, library := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		libraryName, libraryVersion, _ := strings.Cut(library, ":")
		result[ApmLibLanguage(libraryName)] = ApmLibVersion(libraryVersion)
	}
	return result
}

// parseAPMLanguagesValue parses the DD_APM_INSTRUMENTATION_LANGUAGES value.
func parseAPMLanguagesValue(value string) map[ApmLibLanguage]ApmLibVersion {
	res := map[ApmLibLanguage]ApmLibVersion{}
	for language := range strings.SplitSeq(value, " ") {
		if len(language) > 0 {
			res[ApmLibLanguage(language)] = ""
		}
	}
	return res
}

// ApmLibLanguage is a language defined in DD_APM_INSTRUMENTATION_LIBRARIES env var
type ApmLibLanguage string

// ApmLibVersion is the version of the library defined in DD_APM_INSTRUMENTATION_LIBRARIES env var
type ApmLibVersion string

const (
	// APMInstrumentationEnabledAll enables APM instrumentation for all containers.
	APMInstrumentationEnabledAll = "all"
	// APMInstrumentationEnabledDocker enables APM instrumentation for Docker containers.
	APMInstrumentationEnabledDocker = "docker"
	// APMInstrumentationEnabledHost enables APM instrumentation for the host.
	APMInstrumentationEnabledHost = "host"
	// APMInstrumentationEnabledIIS enables APM instrumentation for .NET applications running on IIS on Windows
	APMInstrumentationEnabledIIS = "iis"
	// APMInstrumentationNotSet is the default value when the environment variable is not set.
	APMInstrumentationNotSet = "not_set"
)

// MsiParamsEnv contains the environment variables for options that are passed to the MSI.
type MsiParamsEnv struct {
	AgentUserName            string
	AgentUserPassword        string
	ProjectLocation          string
	ApplicationDataDirectory string
}

// InstallScriptEnv contains the environment variables for the install script.
type InstallScriptEnv struct {
	// SSI
	APMInstrumentationEnabled string

	// APM features toggles
	RuntimeMetricsEnabled       *bool
	LogsInjection               *bool
	APMTracingEnabled           *bool
	ProfilingEnabled            string
	DataStreamsEnabled          *bool
	AppsecEnabled               *bool
	IastEnabled                 *bool
	DataJobsEnabled             *bool
	AppsecScaEnabled            *bool
	TracerLogsCollectionEnabled *bool

	// RUM configuration
	RumEnabled               *bool
	RumApplicationID         string
	RumClientToken           string
	RumRemoteConfigurationID string
	RumSite                  string
}

// ExtensionRegistryOverride holds registry settings for a single extension.
type ExtensionRegistryOverride struct {
	URL      string
	Auth     string
	Username string
	Password string
}

// Env contains the configuration for the installer.
type Env struct {
	APIKey               string
	Site                 string
	RemoteUpdates        bool
	OTelCollectorEnabled bool
	ConfigID             string

	Mirror                      string
	RegistryOverride            string
	RegistryAuthOverride        string
	RegistryUsername            string
	RegistryPassword            string
	RegistryOverrideByImage     map[string]string
	RegistryAuthOverrideByImage map[string]string
	RegistryUsernameByImage     map[string]string
	RegistryPasswordByImage     map[string]string

	DefaultPackagesInstallOverride map[string]bool
	DefaultPackagesVersionOverride map[string]string

	ApmLibraries map[ApmLibLanguage]ApmLibVersion

	AgentMajorVersion string
	AgentMinorVersion string

	MsiParams MsiParamsEnv // windows only

	InstallScript InstallScriptEnv

	Tags     []string
	Hostname string

	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string

	InfrastructureMode string

	AppKey              string
	PAREnabled          bool
	PARActionsAllowlist string

	IsCentos6 bool

	IsFromDaemon bool

	LogLevel string

	// ExtensionRegistryOverrides holds per-package, per-extension registry
	// overrides parsed from installer.registry.extensions in datadog.yaml.
	// Outer key is package name, inner key is extension name.
	ExtensionRegistryOverrides map[string]map[string]ExtensionRegistryOverride
}

// HTTPClient returns an HTTP client with the proxy settings from the environment.
func (e *Env) HTTPClient() *http.Client {
	proxyConfig := &httpproxy.Config{
		HTTPProxy:  e.HTTPProxy,
		HTTPSProxy: e.HTTPSProxy,
		NoProxy:    e.NoProxy,
	}
	proxyFunc := func(r *http.Request) (*url.URL, error) {
		return proxyConfig.ProxyFunc()(r.URL)
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			Proxy:                 proxyFunc,
		},
	}
	return client
}

type Option func(*options)
type options struct {
	configDir string
}

// WithConfigDir overrides the directory where datadog.yaml is read from.
func WithConfigDir(dir string) Option {
	return func(o *options) {
		o.configDir = dir
	}
}

// Get returns an Env struct with values resolved using the priority chain:
//
//	defaults < config (datadog.yaml) < environment variables < option overrides
//
// By default, datadog.yaml is read from paths.AgentConfigDir.
// Use WithConfigDir to override the config directory (e.g. in tests).
func Get(opts ...Option) *Env {
	o := &options{configDir: paths.AgentConfigDir}
	for _, opt := range opts {
		opt(o)
	}

	// 1. Start from defaults
	env := newDefaultEnv()

	// 2. Config layer
	if cfg := readDatadogYAML(o.configDir); cfg != nil {
		applyConfig(&env, cfg)
	}

	// 3. Env var table
	for _, b := range envBindings {
		if b.prefixEnvVar {
			prefix := b.envVar + "_"
			for _, kv := range os.Environ() {
				key, val, ok := strings.Cut(kv, "=")
				if !ok {
					continue
				}
				if suffix, found := strings.CutPrefix(key, prefix); found {
					suffix = strings.ToLower(suffix)
					suffix = strings.ReplaceAll(suffix, "_", "-")
					b.fromOs(&env, suffix+"="+val)
				}
			}
		} else if value, ok := os.LookupEnv(b.envVar); ok {
			b.fromOs(&env, value)
		}
	}

	// 4. System detection
	env.IsCentos6 = DetectCentos6()

	return &env
}

// ToEnv returns a slice of environment variables from the Env struct.
func (e *Env) ToEnv() []string {
	var env []string
	for _, b := range envBindings {
		if b.prefixEnvVar {
			if b.toOsPrefix == nil {
				continue
			}
			for suffix, val := range b.toOsPrefix(e) {
				suffix = strings.ReplaceAll(suffix, "-", "_")
				suffix = strings.ToUpper(suffix)
				env = append(env, b.envVar+"_"+suffix+"="+val)
			}
		} else if b.toOs != nil {
			if v := b.toOs(e); v != "" {
				env = append(env, b.envVar+"="+v)
			}
		}
	}
	return env
}

// DetectCentos6 checks if the machine the installer is currently on is running centos 6
func DetectCentos6() bool {
	sources := []string{
		"/etc/system-release",
		"/etc/centos-release",
		"/etc/redhat-release",
	}
	for _, s := range sources {
		b, _ := os.ReadFile(s)
		if (bytes.Contains(b, []byte("CentOS")) || bytes.Contains(b, []byte("Red Hat"))) &&
			bytes.Contains(b, []byte("release 6")) {
			return true
		}
	}
	return false
}

// ValidateAPMInstrumentationEnabled validates the value of the DD_APM_INSTRUMENTATION_ENABLED environment variable.
func ValidateAPMInstrumentationEnabled(value string) error {
	if value != APMInstrumentationEnabledAll && value != APMInstrumentationEnabledDocker && value != APMInstrumentationEnabledHost && value != APMInstrumentationNotSet {
		return fmt.Errorf("invalid value for %s: %s", envApmInstrumentationEnabled, value)
	}
	return nil
}

// GetAgentVersion returns the agent version from the environment variables.
func (e *Env) GetAgentVersion() string {
	minorVersion := e.AgentMinorVersion
	if strings.Contains(minorVersion, ".") && !strings.HasSuffix(minorVersion, "-1") {
		minorVersion = minorVersion + "-1"
	}
	if e.AgentMajorVersion != "" && minorVersion != "" {
		return e.AgentMajorVersion + "." + minorVersion
	}
	if minorVersion != "" {
		return "7." + minorVersion
	}
	return "latest"
}
