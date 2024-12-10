// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package env provides the environment variables for the installer.
package env

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

const (
	envAPIKey                = "DD_API_KEY"
	envSite                  = "DD_SITE"
	envRemoteUpdates         = "DD_REMOTE_UPDATES"
	envRemotePolicies        = "DD_REMOTE_POLICIES"
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
	envCDNLocalDirPath       = "DD_INSTALLER_DEBUG_CDN_LOCAL_DIR_PATH"
	envCDNEnabled            = "DD_INSTALLER_CDN_ENABLED"
	envAgentUserName         = "DD_AGENT_USER_NAME"
	// envAgentUserNameCompat provides compatibility with the original MSI parameter name
	envAgentUserNameCompat = "DDAGENTUSER_NAME"
	envTags                = "DD_TAGS"
	envExtraTags           = "DD_EXTRA_TAGS"
	envHostname            = "DD_HOSTNAME"
	envDDHTTPProxy         = "DD_PROXY_HTTP"
	envHTTPProxy           = "HTTP_PROXY"
	envDDHTTPSProxy        = "DD_PROXY_HTTPS"
	envHTTPSProxy          = "HTTPS_PROXY"
	envDDNoProxy           = "DD_PROXY_NO_PROXY"
	envNoProxy             = "NO_PROXY"

	// install script
	envApmInstrumentationEnabled = "DD_APM_INSTRUMENTATION_ENABLED"
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
	APMInstrumentationEnabled string
}

// Env contains the configuration for the installer.
type Env struct {
	APIKey         string
	Site           string
	RemoteUpdates  bool
	RemotePolicies bool

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

	InstallScript InstallScriptEnv

	CDNEnabled      bool
	CDNLocalDirPath string

	Tags     []string
	Hostname string

	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
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
		APIKey:         getEnvOrDefault(envAPIKey, defaultEnv.APIKey),
		Site:           getEnvOrDefault(envSite, defaultEnv.Site),
		RemoteUpdates:  strings.ToLower(os.Getenv(envRemoteUpdates)) == "true",
		RemotePolicies: strings.ToLower(os.Getenv(envRemotePolicies)) == "true",

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

		InstallScript: InstallScriptEnv{
			APMInstrumentationEnabled: getEnvOrDefault(envApmInstrumentationEnabled, APMInstrumentationNotSet),
		},

		CDNEnabled:      strings.ToLower(os.Getenv(envCDNEnabled)) == "true",
		CDNLocalDirPath: getEnvOrDefault(envCDNLocalDirPath, ""),

		Tags: append(
			strings.FieldsFunc(os.Getenv(envTags), splitFunc),
			strings.FieldsFunc(os.Getenv(envExtraTags), splitFunc)...,
		),
		Hostname: os.Getenv(envHostname),

		HTTPProxy:  getProxySetting(envDDHTTPProxy, envHTTPProxy),
		HTTPSProxy: getProxySetting(envDDHTTPSProxy, envHTTPSProxy),
		NoProxy:    getProxySetting(envDDNoProxy, envNoProxy),
	}
}

// ToEnv returns a slice of environment variables from the Env struct.
func (e *Env) ToEnv() []string {
	var env []string
	if e.APIKey != "" {
		env = append(env, envAPIKey+"="+e.APIKey)
	}
	if e.Site != "" {
		env = append(env, envSite+"="+e.Site)
	}
	if e.RemoteUpdates {
		env = append(env, envRemoteUpdates+"=true")
	}
	if e.RemotePolicies {
		env = append(env, envRemotePolicies+"=true")
	}
	if e.Mirror != "" {
		env = append(env, envMirror+"="+e.Mirror)
	}
	if e.RegistryOverride != "" {
		env = append(env, envRegistryURL+"="+e.RegistryOverride)
	}
	if e.RegistryAuthOverride != "" {
		env = append(env, envRegistryAuth+"="+e.RegistryAuthOverride)
	}
	if e.RegistryUsername != "" {
		env = append(env, envRegistryUsername+"="+e.RegistryUsername)
	}
	if e.RegistryPassword != "" {
		env = append(env, envRegistryPassword+"="+e.RegistryPassword)
	}
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
	if len(e.Hostname) > 0 {
		env = append(env, envHostname+"="+e.Hostname)
	}
	if e.HTTPProxy != "" {
		env = append(env, envHTTPProxy+"="+e.HTTPProxy)
	}
	if e.HTTPSProxy != "" {
		env = append(env, envHTTPSProxy+"="+e.HTTPSProxy)
	}
	if e.NoProxy != "" {
		env = append(env, envNoProxy+"="+e.NoProxy)
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
	value := os.Getenv(env)
	if value == "" {
		return defaultValue
	}
	return value
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
