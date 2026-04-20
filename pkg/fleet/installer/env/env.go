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
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
	"golang.org/x/net/http/httpproxy"
)

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
	AgentUserName            string `env:"DDAGENTUSER_NAME,DD_AGENT_USER_NAME"`
	AgentUserPassword        string `env:"DDAGENTUSER_PASSWORD,DD_AGENT_USER_PASSWORD"`
	ProjectLocation          string `env:"DD_PROJECTLOCATION"`
	ApplicationDataDirectory string `env:"DD_APPLICATIONDATADIRECTORY"`
}

// InstallScriptEnv contains the environment variables for the install script.
type InstallScriptEnv struct {
	// SSI
	APMInstrumentationEnabled string `env:"DD_APM_INSTRUMENTATION_ENABLED"`

	// APM features toggles
	RuntimeMetricsEnabled       *bool  `env:"DD_RUNTIME_METRICS_ENABLED"`
	LogsInjection               *bool  `env:"DD_LOGS_INJECTION"`
	APMTracingEnabled           *bool  `env:"DD_APM_TRACING_ENABLED"`
	ProfilingEnabled            string `env:"DD_PROFILING_ENABLED"`
	DataStreamsEnabled          *bool  `env:"DD_DATA_STREAMS_ENABLED"`
	AppsecEnabled               *bool  `env:"DD_APPSEC_ENABLED"`
	IastEnabled                 *bool  `env:"DD_IAST_ENABLED"`
	DataJobsEnabled             *bool  `env:"DD_DATA_JOBS_ENABLED"`
	AppsecScaEnabled            *bool  `env:"DD_APPSEC_SCA_ENABLED"`
	TracerLogsCollectionEnabled *bool  `env:"DD_APP_LOGS_COLLECTION_ENABLED"`

	// RUM configuration
	RumEnabled               *bool  `env:"DD_RUM_ENABLED"`
	RumApplicationID         string `env:"DD_RUM_APPLICATION_ID"`
	RumClientToken           string `env:"DD_RUM_CLIENT_TOKEN"`
	RumRemoteConfigurationID string `env:"DD_RUM_REMOTE_CONFIGURATION_ID"`
	RumSite                  string `env:"DD_RUM_SITE"`
}

// ExtensionRegistryOverride holds registry settings for a single extension.
type ExtensionRegistryOverride struct {
	URL      string
	Auth     string
	Username string
	Password string
}

// Env contains the configuration for the installer.
//
// Priority order:
// Environment variables > Config (datadog.yaml) > Defaults
//
// Fields can declare where they are read from using struct tags:
//
//	env:"<VAR>[,<VAR2>,...][,prefix]"   - environment variable(s)
//	yaml:"<dotted.path>"                - dotted key in datadog.yaml
//
// For env: vars are processed in declaration order (last set wins). The
// optional ",prefix" marker turns the field into a prefix-scan map (reads
// env vars shaped like `<VAR>_<SUFFIX>=<value>`).
// For yaml: the tag names a dotted path into the parsed yaml tree; if that
// path resolves to a non-zero scalar, the field is set from it.
// Specific per-env-var behaviour that deviates from the type-driven default
// lives as a named case in the switch inside applyEnvTags / ToEnv. The
// ExtensionRegistryOverrides nested map is populated by a bespoke branch in
// applyConfig because its shape doesn't fit the scalar walker.
type Env struct {
	APIKey               string `env:"DD_API_KEY" yaml:"api_key"`
	Site                 string `env:"DD_SITE" yaml:"site"`
	RemoteUpdates        bool   `env:"DD_REMOTE_UPDATES"`
	OTelCollectorEnabled bool   `env:"DD_OTELCOLLECTOR_ENABLED"`
	ConfigID             string

	Mirror                      string            `env:"DD_INSTALLER_MIRROR"`
	RegistryOverride            string            `env:"DD_INSTALLER_REGISTRY_URL" yaml:"installer.registry.url"`
	RegistryAuthOverride        string            `env:"DD_INSTALLER_REGISTRY_AUTH" yaml:"installer.registry.auth"`
	RegistryUsername            string            `env:"DD_INSTALLER_REGISTRY_USERNAME" yaml:"installer.registry.username"`
	RegistryPassword            string            `env:"DD_INSTALLER_REGISTRY_PASSWORD" yaml:"installer.registry.password"`
	RegistryOverrideByImage     map[string]string `env:"DD_INSTALLER_REGISTRY_URL,prefix"`
	RegistryAuthOverrideByImage map[string]string `env:"DD_INSTALLER_REGISTRY_AUTH,prefix"`
	RegistryUsernameByImage     map[string]string `env:"DD_INSTALLER_REGISTRY_USERNAME,prefix"`
	RegistryPasswordByImage     map[string]string `env:"DD_INSTALLER_REGISTRY_PASSWORD,prefix"`

	DefaultPackagesInstallOverride map[string]bool   `env:"DD_INSTALLER_DEFAULT_PKG_INSTALL,prefix"`
	DefaultPackagesVersionOverride map[string]string `env:"DD_INSTALLER_DEFAULT_PKG_VERSION,prefix"`

	ApmLibraries map[ApmLibLanguage]ApmLibVersion `env:"DD_APM_INSTRUMENTATION_LANGUAGES,DD_APM_INSTRUMENTATION_LIBRARIES"`

	AgentMajorVersion string `env:"DD_AGENT_MAJOR_VERSION"`
	AgentMinorVersion string `env:"DD_AGENT_MINOR_VERSION"`

	MsiParams MsiParamsEnv // windows only

	InstallScript InstallScriptEnv

	Tags     []string `env:"DD_TAGS,DD_EXTRA_TAGS"`
	Hostname string   `env:"DD_HOSTNAME"`

	HTTPProxy  string `env:"http_proxy,HTTP_PROXY,DD_PROXY_HTTP"`
	HTTPSProxy string `env:"https_proxy,HTTPS_PROXY,DD_PROXY_HTTPS"`
	NoProxy    string `env:"no_proxy,NO_PROXY,DD_PROXY_NO_PROXY"`

	InfrastructureMode string `env:"DD_INFRASTRUCTURE_MODE"`

	AppKey              string `env:"DD_APP_KEY"`
	PAREnabled          bool   `env:"DD_PRIVATE_ACTION_RUNNER_ENABLED"`
	PARActionsAllowlist string `env:"DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST"`

	IsCentos6 bool

	IsFromDaemon bool `env:"DD_INSTALLER_FROM_DAEMON"`

	LogLevel string `env:"DD_LOG_LEVEL"`

	// ExtensionRegistryOverrides holds per-package, per-extension registry
	// overrides parsed from installer.registry.extensions in datadog.yaml.
	// Outer key is package name, inner key is extension name.
	ExtensionRegistryOverrides map[string]map[string]ExtensionRegistryOverride `yaml:"installer.registry.extensions"`
}

func newDefaultEnv() Env {
	return Env{
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

type envField struct {
	vars      []string
	prefix    bool
	indexPath []int // reflect.Value.FieldByIndex path starting at the Env struct
}

// envFields returns the list of tagged fields across Env and its
// sub-structs (MsiParamsEnv, InstallScriptEnv).
var envFields = sync.OnceValue(func() []envField {
	var out []envField
	collectEnvFields(reflect.TypeOf(Env{}), nil, &out)
	return out
})

func collectEnvFields(t reflect.Type, indexPath []int, out *[]envField) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		path := append(append([]int{}, indexPath...), i)
		tag := f.Tag.Get("env")
		if tag == "" {
			if f.Type.Kind() == reflect.Struct {
				collectEnvFields(f.Type, path, out)
			}
			continue
		}
		parts := strings.Split(tag, ",")
		isPrefix := false
		vars := make([]string, 0, len(parts))
		for _, p := range parts {
			if p == "prefix" {
				isPrefix = true
				continue
			}
			vars = append(vars, p)
		}
		*out = append(*out, envField{
			vars:      vars,
			prefix:    isPrefix,
			indexPath: path,
		})
	}
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

// defaultConfigDir is the directory where datadog.yaml is read from by default.
// It is populated by paths.init() via SetDefaultConfigDir to avoid an import
// cycle between env and paths.
var defaultConfigDir string

// SetDefaultConfigDir registers the default directory where datadog.yaml is
// looked up when WithConfigDir is not supplied. It is intended to be called by
// the paths package at init time.
func SetDefaultConfigDir(dir string) {
	defaultConfigDir = dir
}

// WithConfigDir overrides the directory where datadog.yaml is read from.
func WithConfigDir(dir string) Option {
	return func(o *options) {
		o.configDir = dir
	}
}

// Get returns an Env struct with values resolved using the priority chain:
//
//	defaults < config (datadog.yaml) < environment variables
//
// By default, datadog.yaml is read from the directory registered via
// SetDefaultConfigDir (populated by the paths package).
// Use WithConfigDir to override the config directory (e.g. in tests).
func Get(opts ...Option) *Env {
	o := &options{configDir: defaultConfigDir}
	for _, opt := range opts {
		opt(o)
	}

	// 1. Start from defaults
	env := newDefaultEnv()

	// 2. Config layer
	if cfg := readDatadogYAML(o.configDir); cfg != nil {
		applyConfig(&env, cfg)
	}

	// 3. Environment variables layer
	applyEnvTags(&env)

	// 4. Centos 6 detection
	env.IsCentos6 = DetectCentos6()

	return &env
}

// readDatadogYAML reads datadog.yaml from configDir and unmarshals it into a
// generic map. Returns nil if the file is missing or unparseable (best
// effort — yaml is optional).
func readDatadogYAML(configDir string) map[string]any {
	if configDir == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(configDir, "datadog.yaml"))
	if err != nil {
		return nil
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	return cfg
}

// applyConfig walks Env fields and, for every field declaring a `yaml` tag,
// looks the tag's dotted path up in cfg. Non-zero scalar values overwrite the
// default. ExtensionRegistryOverrides is handled by a bespoke branch because
// its shape (map-of-maps-of-structs) doesn't fit the scalar walker.
func applyConfig(env *Env, cfg map[string]any) {
	walkYAMLConfig(reflect.ValueOf(env).Elem(), cfg)
}

func walkYAMLConfig(v reflect.Value, cfg map[string]any) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		yamlPath := f.Tag.Get("yaml")
		if yamlPath == "" {
			if f.Type.Kind() == reflect.Struct {
				walkYAMLConfig(v.Field(i), cfg)
			}
			continue
		}
		raw := lookupYAMLPath(cfg, yamlPath)
		if raw == nil {
			continue
		}
		setFieldFromYAML(v.Field(i), raw)
	}
}

func lookupYAMLPath(cfg map[string]any, path string) any {
	var cur any = cfg
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[seg]
		if !ok {
			return nil
		}
	}
	return cur
}

func setFieldFromYAML(field reflect.Value, raw any) {
	switch field.Kind() {
	case reflect.String:
		if s, ok := raw.(string); ok && s != "" {
			field.SetString(s)
		}
	case reflect.Bool:
		if b, ok := raw.(bool); ok {
			field.SetBool(b)
		}
	case reflect.Map:
		if field.Type() == reflect.TypeOf((map[string]map[string]ExtensionRegistryOverride)(nil)) {
			applyExtensionOverrides(field, raw)
		}
	}
}

// applyExtensionOverrides copies installer.registry.extensions.<pkg>.<ext>.*
// entries from the yaml tree into the target map field.
func applyExtensionOverrides(field reflect.Value, raw any) {
	exts, ok := raw.(map[string]any)
	if !ok || len(exts) == 0 {
		return
	}
	if field.IsNil() {
		field.Set(reflect.MakeMap(field.Type()))
	}
	overrides := field.Interface().(map[string]map[string]ExtensionRegistryOverride)
	for pkg, extMapAny := range exts {
		extMap, ok := extMapAny.(map[string]any)
		if !ok || len(extMap) == 0 {
			continue
		}
		if overrides[pkg] == nil {
			overrides[pkg] = make(map[string]ExtensionRegistryOverride, len(extMap))
		}
		for extName, extCfgAny := range extMap {
			extCfg, ok := extCfgAny.(map[string]any)
			if !ok {
				continue
			}
			o := overrides[pkg][extName]
			if s, ok := extCfg["url"].(string); ok && s != "" {
				o.URL = s
			}
			if s, ok := extCfg["auth"].(string); ok && s != "" {
				o.Auth = s
			}
			if s, ok := extCfg["username"].(string); ok && s != "" {
				o.Username = s
			}
			if s, ok := extCfg["password"].(string); ok && s != "" {
				o.Password = s
			}
			overrides[pkg][extName] = o
		}
	}
}

// applyEnvTags iterates every tagged field on Env (and its sub-structs) and
// reads its declared environment variables.
func applyEnvTags(env *Env) {
	envValue := reflect.ValueOf(env).Elem()
	for _, f := range envFields() {
		field := envValue.FieldByIndex(f.indexPath)
		if f.prefix {
			// All prefix-scan fields share the same generic read rule —
			// scan os.Environ() for <VAR>_<SUFFIX>=<value>, lowercase the
			// suffix and store into the map. No special cases today.
			readPrefixEnvVar(field, f.vars[0])
			continue
		}
		for _, envVar := range f.vars {
			switch envVar {
			case "DD_API_KEY":
				if v, ok := os.LookupEnv(envVar); ok && v != "" {
					env.APIKey = v
				}
			case "DD_EXTRA_TAGS":
				if v, ok := os.LookupEnv(envVar); ok {
					env.Tags = append(env.Tags, parseCSVTags(v)...)
				}
			case "DD_APM_INSTRUMENTATION_LANGUAGES":
				if v, ok := os.LookupEnv(envVar); ok && v != "" {
					env.ApmLibraries = parseAPMLanguagesValue(v)
				}
			case "DD_APM_INSTRUMENTATION_LIBRARIES":
				if v, ok := os.LookupEnv(envVar); ok {
					env.ApmLibraries = parseApmLibrariesValue(v)
				}
			default:
				if v, ok := os.LookupEnv(envVar); ok {
					setFieldFromEnv(field, v)
				}
			}
		}
	}
}

// ToEnv returns a slice of environment variables from the Env struct.
func (e *Env) ToEnv() []string {
	var out []string
	envValue := reflect.ValueOf(e).Elem()
	for _, f := range envFields() {
		field := envValue.FieldByIndex(f.indexPath)
		if f.prefix {
			emitPrefixEnvVar(&out, field, f.vars[0])
			continue
		}
		for _, envVar := range f.vars {
			switch envVar {
			// Read-only env vars (never emitted)
			case "DD_AGENT_MAJOR_VERSION", "DD_AGENT_MINOR_VERSION",
				"DD_APP_LOGS_COLLECTION_ENABLED", "DD_EXTRA_TAGS",
				"DD_APM_INSTRUMENTATION_LANGUAGES",
				"http_proxy", "https_proxy", "no_proxy",
				"DD_PROXY_HTTP", "DD_PROXY_HTTPS", "DD_PROXY_NO_PROXY",
				"DDAGENTUSER_NAME", "DDAGENTUSER_PASSWORD":
				// skip
			case "DD_APM_INSTRUMENTATION_LIBRARIES":
				if s := formatApmLibraries(e.ApmLibraries); s != "" {
					out = append(out, envVar+"="+s)
				}
			case "DD_APP_KEY":
				if e.PAREnabled && e.AppKey != "" {
					out = append(out, envVar+"="+e.AppKey)
				}
			case "DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST":
				if e.PAREnabled && e.PARActionsAllowlist != "" {
					out = append(out, envVar+"="+e.PARActionsAllowlist)
				}
			case "DD_LOG_LEVEL":
				if e.IsFromDaemon {
					out = append(out, envVar+"=off")
				}
			default:
				if v := formatFieldForEnv(field); v != "" {
					out = append(out, envVar+"="+v)
				}
			}
		}
	}
	return out
}

func parseCSVTags(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, t := range parts {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func setFieldFromEnv(field reflect.Value, v string) {
	switch field.Kind() {
	case reflect.String:
		field.SetString(v)
	case reflect.Bool:
		field.SetBool(strings.ToLower(v) == "true")
	case reflect.Pointer:
		if field.Type().Elem().Kind() == reflect.Bool {
			if b := parseBoolPtr(v); b != nil {
				field.Set(reflect.ValueOf(b))
			}
		}
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			field.Set(reflect.ValueOf(parseCSVTags(v)))
		}
	}
}

func formatFieldForEnv(field reflect.Value) string {
	switch field.Kind() {
	case reflect.String:
		return field.String()
	case reflect.Bool:
		if field.Bool() {
			return "true"
		}
		return ""
	case reflect.Pointer:
		if field.Type().Elem().Kind() == reflect.Bool {
			if field.IsNil() {
				return ""
			}
			return strconv.FormatBool(field.Elem().Bool())
		}
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			n := field.Len()
			if n == 0 {
				return ""
			}
			parts := make([]string, n)
			for i := range n {
				parts[i] = field.Index(i).String()
			}
			return strings.Join(parts, ",")
		}
	}
	return ""
}

// readPrefixEnvVar scans os.Environ() for <envVar>_<SUFFIX>=<value> entries
// and writes them into the map at field.
func readPrefixEnvVar(field reflect.Value, envVar string) {
	if field.Kind() != reflect.Map {
		return
	}
	if field.IsNil() {
		field.Set(reflect.MakeMap(field.Type()))
	}
	keyT := field.Type().Key()
	valT := field.Type().Elem()
	prefix := envVar + "_"
	for _, kv := range os.Environ() {
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		suffix, found := strings.CutPrefix(key, prefix)
		if !found {
			continue
		}
		suffix = strings.ToLower(suffix)
		suffix = strings.ReplaceAll(suffix, "_", "-")
		k := reflect.ValueOf(suffix).Convert(keyT)
		switch valT.Kind() {
		case reflect.String:
			field.SetMapIndex(k, reflect.ValueOf(val).Convert(valT))
		case reflect.Bool:
			field.SetMapIndex(k, reflect.ValueOf(strings.ToLower(val) == "true"))
		}
	}
}

// emitPrefixEnvVar emits <envVar>_<SUFFIX>=<value> for each entry in the map.
// For map[string]bool only true values are emitted (matches today's behaviour
// where false entries don't appear in the child-process env).
func emitPrefixEnvVar(out *[]string, field reflect.Value, envVar string) {
	if field.Kind() != reflect.Map {
		return
	}
	valKind := field.Type().Elem().Kind()
	iter := field.MapRange()
	for iter.Next() {
		suffix := iter.Key().String()
		var val string
		switch valKind {
		case reflect.String:
			val = iter.Value().String()
		case reflect.Bool:
			if !iter.Value().Bool() {
				continue
			}
			val = "true"
		default:
			continue
		}
		suffix = strings.ToUpper(strings.ReplaceAll(suffix, "-", "_"))
		*out = append(*out, envVar+"_"+suffix+"="+val)
	}
}

// formatApmLibraries renders ApmLibraries as a sorted "lang[:version],…"
// string for DD_APM_INSTRUMENTATION_LIBRARIES.
func formatApmLibraries(libs map[ApmLibLanguage]ApmLibVersion) string {
	if len(libs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(libs))
	for l, v := range libs {
		s := string(l)
		if v != "" {
			s += ":" + string(v)
		}
		parts = append(parts, s)
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
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
		return fmt.Errorf("invalid value for %s: %s", "DD_APM_INSTRUMENTATION_ENABLED", value)
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
