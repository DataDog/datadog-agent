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

	"golang.org/x/net/http/httpproxy"
)

const (
	envAPIKey                = "DD_API_KEY"
	envSite                  = "DD_SITE"
	envRemoteUpdates         = "DD_REMOTE_UPDATES"
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
	envAgentUserName         = "DD_AGENT_USER_NAME"
	// envAgentUserNameCompat provides compatibility with the original MSI parameter name
	envAgentUserNameCompat = "DDAGENTUSER_NAME"
	envAgentUserPassword   = "DD_AGENT_USER_PASSWORD"
	// envAgentUserPasswordCompat provides compatibility with the original MSI parameter name
	envAgentUserPasswordCompat = "DDAGENTUSER_PASSWORD"
	envTags                    = "DD_TAGS"
	envExtraTags               = "DD_EXTRA_TAGS"
	envHostname                = "DD_HOSTNAME"
	envDDHTTPProxy             = "DD_PROXY_HTTP"
	envHTTPProxy               = "HTTP_PROXY"
	envDDHTTPSProxy            = "DD_PROXY_HTTPS"
	envHTTPSProxy              = "HTTPS_PROXY"
	envDDNoProxy               = "DD_PROXY_NO_PROXY"
	envNoProxy                 = "NO_PROXY"
	envIsFromDaemon            = "DD_INSTALLER_FROM_DAEMON"

	// install script
	envApmInstrumentationEnabled = "DD_APM_INSTRUMENTATION_ENABLED"
	envRuntimeMetricsEnabled     = "DD_RUNTIME_METRICS_ENABLED"
	envLogsInjection             = "DD_LOGS_INJECTION"
	envAPMTracingEnabled         = "DD_APM_TRACING_ENABLED"
	envProfilingEnabled          = "DD_PROFILING_ENABLED"
	envDataStreamsEnabled        = "DD_DATA_STREAMS_ENABLED"
	envAppsecEnabled             = "DD_APPSEC_ENABLED"
	envIastEnabled               = "DD_IAST_ENABLED"
	envDataJobsEnabled           = "DD_DATA_JOBS_ENABLED"
	envAppsecScaEnabled          = "DD_APPSEC_SCA_ENABLED"
)

var defaultEnv = Env{
	APIKey:        "",
	Site:          "datadoghq.com",
	RemoteUpdates: false,
	Mirror:        "",

	RegistryOverride:            "",
	RegistryAuthOverride:        "",
	RegistryUsername:            "",
	RegistryPassword:            "",
	RegistryOverrideByImage:     map[string]string{},
	RegistryAuthOverrideByImage: map[string]string{},
	RegistryUsernameByImage:     map[string]string{},
	RegistryPasswordByImage:     map[string]string{},

	DefaultPackagesInstallOverride: map[string]bool{},
	DefaultPackagesVersionOverride: map[string]string{},

	InstallScript: InstallScriptEnv{
		APMInstrumentationEnabled: "",
		RuntimeMetricsEnabled:     nil,
		LogsInjection:             nil,
		APMTracingEnabled:         nil,
		ProfilingEnabled:          "",
		DataStreamsEnabled:        nil,
		AppsecEnabled:             nil,
		IastEnabled:               nil,
		DataJobsEnabled:           nil,
		AppsecScaEnabled:          nil,
	},
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
	// APMInstrumentationNotSet is the default value when the environment variable is not set.
	APMInstrumentationNotSet = "not_set"
)

// InstallScriptEnv contains the environment variables for the install script.
type InstallScriptEnv struct {
	// SSI
	APMInstrumentationEnabled string

	// APM features toggles
	RuntimeMetricsEnabled *bool
	LogsInjection         *bool
	APMTracingEnabled     *bool
	ProfilingEnabled      string
	DataStreamsEnabled    *bool
	AppsecEnabled         *bool
	IastEnabled           *bool
	DataJobsEnabled       *bool
	AppsecScaEnabled      *bool
}

// Env contains the configuration for the installer.
type Env struct {
	APIKey        string
	Site          string
	RemoteUpdates bool

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
	AgentUserName     string // windows only
	AgentUserPassword string // windows only

	InstallScript InstallScriptEnv

	Tags     []string
	Hostname string

	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string

	IsCentos6 bool

	IsFromDaemon bool
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

// FromEnv returns an Env struct with values from the environment.
func FromEnv() *Env {
	splitFunc := func(c rune) bool {
		return c == ','
	}

	return &Env{
		APIKey:        getEnvOrDefault(envAPIKey, defaultEnv.APIKey),
		Site:          getEnvOrDefault(envSite, defaultEnv.Site),
		RemoteUpdates: strings.ToLower(os.Getenv(envRemoteUpdates)) == "true",

		Mirror:                      getEnvOrDefault(envMirror, defaultEnv.Mirror),
		RegistryOverride:            getEnvOrDefault(envRegistryURL, defaultEnv.RegistryOverride),
		RegistryAuthOverride:        getEnvOrDefault(envRegistryAuth, defaultEnv.RegistryAuthOverride),
		RegistryUsername:            getEnvOrDefault(envRegistryUsername, defaultEnv.RegistryUsername),
		RegistryPassword:            getEnvOrDefault(envRegistryPassword, defaultEnv.RegistryPassword),
		RegistryOverrideByImage:     overridesByNameFromEnv(envRegistryURL, func(s string) string { return s }),
		RegistryAuthOverrideByImage: overridesByNameFromEnv(envRegistryAuth, func(s string) string { return s }),
		RegistryUsernameByImage:     overridesByNameFromEnv(envRegistryUsername, func(s string) string { return s }),
		RegistryPasswordByImage:     overridesByNameFromEnv(envRegistryPassword, func(s string) string { return s }),

		DefaultPackagesInstallOverride: overridesByNameFromEnv(envDefaultPackageInstall, func(s string) bool { return strings.ToLower(s) == "true" }),
		DefaultPackagesVersionOverride: overridesByNameFromEnv(envDefaultPackageVersion, func(s string) string { return s }),

		ApmLibraries: parseApmLibrariesEnv(),

		AgentMajorVersion: os.Getenv(envAgentMajorVersion),
		AgentMinorVersion: os.Getenv(envAgentMinorVersion),
		AgentUserName:     getEnvOrDefault(envAgentUserName, os.Getenv(envAgentUserNameCompat)),
		AgentUserPassword: getEnvOrDefault(envAgentUserPassword, os.Getenv(envAgentUserPasswordCompat)),

		InstallScript: InstallScriptEnv{
			APMInstrumentationEnabled: getEnvOrDefault(envApmInstrumentationEnabled, APMInstrumentationNotSet),
			RuntimeMetricsEnabled:     getBoolEnv(envRuntimeMetricsEnabled),
			LogsInjection:             getBoolEnv(envLogsInjection),
			APMTracingEnabled:         getBoolEnv(envAPMTracingEnabled),
			ProfilingEnabled:          getEnvOrDefault(envProfilingEnabled, ""),
			DataStreamsEnabled:        getBoolEnv(envDataStreamsEnabled),
			AppsecEnabled:             getBoolEnv(envAppsecEnabled),
			IastEnabled:               getBoolEnv(envIastEnabled),
			DataJobsEnabled:           getBoolEnv(envDataJobsEnabled),
			AppsecScaEnabled:          getBoolEnv(envAppsecScaEnabled),
		},

		Tags: append(
			strings.FieldsFunc(os.Getenv(envTags), splitFunc),
			strings.FieldsFunc(os.Getenv(envExtraTags), splitFunc)...,
		),
		Hostname: os.Getenv(envHostname),

		HTTPProxy:  getProxySetting(envDDHTTPProxy, envHTTPProxy),
		HTTPSProxy: getProxySetting(envDDHTTPSProxy, envHTTPSProxy),
		NoProxy:    getProxySetting(envDDNoProxy, envNoProxy),

		IsCentos6:    DetectCentos6(),
		IsFromDaemon: os.Getenv(envIsFromDaemon) == "true",
	}
}

func appendBoolEnv(env []string, key string, value *bool) []string {
	if value != nil {
		env = append(env, key+"="+strconv.FormatBool(*value))
	}
	return env
}

func appendStringEnv(env []string, key string, value string, skipIfEqual string) []string {
	if value != skipIfEqual {
		env = append(env, key+"="+value)
	}
	return env
}

// ToEnv returns a slice of environment variables from the InstallScriptEnv struct
func (e *InstallScriptEnv) ToEnv(env []string) []string {
	env = appendStringEnv(env, envApmInstrumentationEnabled, e.APMInstrumentationEnabled, "")
	env = appendBoolEnv(env, envRuntimeMetricsEnabled, e.RuntimeMetricsEnabled)
	env = appendBoolEnv(env, envLogsInjection, e.LogsInjection)
	env = appendBoolEnv(env, envAPMTracingEnabled, e.APMTracingEnabled)
	env = appendStringEnv(env, envProfilingEnabled, e.ProfilingEnabled, "")
	env = appendBoolEnv(env, envDataStreamsEnabled, e.DataStreamsEnabled)
	env = appendBoolEnv(env, envAppsecEnabled, e.AppsecEnabled)
	env = appendBoolEnv(env, envIastEnabled, e.IastEnabled)
	env = appendBoolEnv(env, envDataJobsEnabled, e.DataJobsEnabled)
	env = appendBoolEnv(env, envAppsecScaEnabled, e.AppsecScaEnabled)
	return env
}

// ToEnv returns a slice of environment variables from the Env struct.
func (e *Env) ToEnv() []string {
	var env []string
	env = appendStringEnv(env, envAPIKey, e.APIKey, "")
	env = appendStringEnv(env, envSite, e.Site, "")
	if e.RemoteUpdates {
		env = append(env, envRemoteUpdates+"=true")
	}
	env = appendStringEnv(env, envMirror, e.Mirror, "")
	env = appendStringEnv(env, envRegistryURL, e.RegistryOverride, "")
	env = appendStringEnv(env, envRegistryAuth, e.RegistryAuthOverride, "")
	env = appendStringEnv(env, envRegistryUsername, e.RegistryUsername, "")
	env = appendStringEnv(env, envRegistryPassword, e.RegistryPassword, "")
	env = e.InstallScript.ToEnv(env)
	if len(e.ApmLibraries) > 0 {
		libraries := []string{}
		for l, v := range e.ApmLibraries {
			l := string(l)
			if v != "" {
				l = l + ":" + string(v)
			}
			libraries = append(libraries, l)
		}
		slices.Sort(libraries)
		env = append(env, envApmLibraries+"="+strings.Join(libraries, ","))
	}
	if len(e.Tags) > 0 {
		env = append(env, envTags+"="+strings.Join(e.Tags, ","))
	}
	env = appendStringEnv(env, envHostname, e.Hostname, "")
	env = appendStringEnv(env, envHTTPProxy, e.HTTPProxy, "")
	env = appendStringEnv(env, envHTTPSProxy, e.HTTPSProxy, "")
	env = appendStringEnv(env, envNoProxy, e.NoProxy, "")
	if e.IsFromDaemon {
		env = append(env, envIsFromDaemon+"=true")
		// This is a bit of a hack; as we should properly redirect the log level
		// to a file or a structured output. But today, we just want to avoid
		// logging to avoid polluting the parsed output.
		// The easiest way to do this without having to import setup/log & pkg/config
		// is by env var.
		env = append(env, "DD_LOG_LEVEL=off")
	}
	env = append(env, overridesByNameToEnv(envRegistryURL, e.RegistryOverrideByImage)...)
	env = append(env, overridesByNameToEnv(envRegistryAuth, e.RegistryAuthOverrideByImage)...)
	env = append(env, overridesByNameToEnv(envRegistryUsername, e.RegistryUsernameByImage)...)
	env = append(env, overridesByNameToEnv(envRegistryPassword, e.RegistryPasswordByImage)...)
	env = append(env, overridesByNameToEnv(envDefaultPackageInstall, e.DefaultPackagesInstallOverride)...)
	env = append(env, overridesByNameToEnv(envDefaultPackageVersion, e.DefaultPackagesVersionOverride)...)
	return env
}

func parseApmLibrariesEnv() map[ApmLibLanguage]ApmLibVersion {
	apmLibraries, ok := os.LookupEnv(envApmLibraries)
	if !ok {
		return parseAPMLanguagesEnv()
	}
	apmLibrariesVersion := map[ApmLibLanguage]ApmLibVersion{}
	if apmLibraries == "" {
		return apmLibrariesVersion
	}
	for _, library := range strings.FieldsFunc(apmLibraries, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		libraryName, libraryVersion, _ := strings.Cut(library, ":")
		apmLibrariesVersion[ApmLibLanguage(libraryName)] = ApmLibVersion(libraryVersion)
	}
	return apmLibrariesVersion
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

func parseAPMLanguagesEnv() map[ApmLibLanguage]ApmLibVersion {
	apmLanguages := os.Getenv(envApmLanguages)
	res := map[ApmLibLanguage]ApmLibVersion{}
	for _, language := range strings.Split(apmLanguages, " ") {
		if len(language) > 0 {
			res[ApmLibLanguage(language)] = ""
		}
	}
	return res
}

func overridesByNameFromEnv[T any](envPrefix string, convert func(string) T) map[string]T {
	env := os.Environ()
	overridesByPackage := map[string]T{}
	for _, e := range env {
		keyVal := strings.SplitN(e, "=", 2)
		if len(keyVal) != 2 {
			continue
		}
		if strings.HasPrefix(keyVal[0], envPrefix+"_") {
			pkg := strings.TrimPrefix(keyVal[0], envPrefix+"_")
			pkg = strings.ToLower(pkg)
			pkg = strings.ReplaceAll(pkg, "_", "-")
			overridesByPackage[pkg] = convert(keyVal[1])
		}
	}
	return overridesByPackage
}

func overridesByNameToEnv[T any](envPrefix string, overridesByPackage map[string]T) []string {
	env := []string{}
	for pkg, override := range overridesByPackage {
		pkg = strings.ReplaceAll(pkg, "-", "_")
		pkg = strings.ToUpper(pkg)
		env = append(env, envPrefix+"_"+pkg+"="+fmt.Sprint(override))
	}
	return env
}

func getEnvOrDefault(env string, defaultValue string) string {
	value, set := os.LookupEnv(env)
	if !set {
		return defaultValue
	}
	return value
}

func getBoolEnv(env string) *bool {
	t := true
	f := false
	value := os.Getenv(env)
	switch value {
	case "true":
		return &t
	case "false":
		return &f
	default:
		return nil
	}
}

func getProxySetting(ddEnv string, env string) string {
	return getEnvOrDefault(
		ddEnv,
		getEnvOrDefault(
			env,
			os.Getenv(strings.ToLower(env)),
		),
	)
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
