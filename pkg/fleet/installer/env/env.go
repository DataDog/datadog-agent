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
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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
// The installer is configured exclusively from environment variables (plus
// CLI flags where they already exist). Any yaml-sourced value must be
// translated to a `DD_*` env var by the caller before invoking the
// installer — this is the daemon's responsibility (via its fx
// config.Component) and the CLI's fx bootstrap.
//
// Fields declare the env vars they listen to via the `env:` struct tag:
//
//	env:"<VAR>[,<VAR2>,...][,prefix]"
//
// Vars are processed in declaration order — the LAST entry in the list
// has the highest precedence. The order is load-bearing; reordering
// entries silently changes precedence, so callers should treat the list
// as stable. The optional ",prefix" marker turns the field into a
// prefix-scan map (reads env vars shaped like `<VAR>_<SUFFIX>=<value>`).
// Specific per-env-var behaviour that deviates from the type-driven
// default lives as a named case in the switch inside applyEnvTags /
// ToEnv. `ExtensionRegistryOverrides` has no single tag because its
// shape requires a dedicated prefix scheme; see
// parseExtensionRegistryEnv / emitExtensionRegistryEnv.
type Env struct {
	APIKey               string `env:"DD_API_KEY"`
	Site                 string `env:"DD_SITE"`
	RemoteUpdates        bool   `env:"DD_REMOTE_UPDATES"`
	OTelCollectorEnabled bool   `env:"DD_OTELCOLLECTOR_ENABLED"`
	ConfigID             string

	Mirror                      string            `env:"DD_INSTALLER_MIRROR"`
	RegistryOverride            string            `env:"DD_INSTALLER_REGISTRY_URL"`
	RegistryAuthOverride        string            `env:"DD_INSTALLER_REGISTRY_AUTH"`
	RegistryUsername            string            `env:"DD_INSTALLER_REGISTRY_USERNAME"`
	RegistryPassword            string            `env:"DD_INSTALLER_REGISTRY_PASSWORD"`
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
	// overrides. Sourced from env vars matching
	// DD_INSTALLER_REGISTRY_EXT_{URL,AUTH,USERNAME,PASSWORD}_<PKG>__<EXT>.
	// Outer key is package name, inner key is extension name (both lowercased
	// with `_`→`-`). Populated by parseExtensionRegistryEnv; the daemon
	// translates the matching yaml section to these env vars before spawning
	// the installer.
	ExtensionRegistryOverrides map[string]map[string]ExtensionRegistryOverride
}

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

		LogLevel: "warn",

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

// Get returns an Env struct resolved from defaults overridden by environment
// variables. The installer never reads datadog.yaml; callers that need
// yaml-sourced values translate them into `DD_*` env vars first (the daemon
// via its fx config.Component, the CLI via its fx config bootstrap).
func Get() *Env {
	env := newDefaultEnv()
	applyEnvTags(&env)
	parseExtensionRegistryEnv(&env)
	env.IsCentos6 = DetectCentos6()
	return &env
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
	emitExtensionRegistryEnv(&out, e.ExtensionRegistryOverrides)
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
				"DD_EXTRA_TAGS",
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

// setFieldFromEnv decodes a raw env-var string into field. Malformed input
// (e.g. non-numeric text for an int field) leaves the field at its default
// rather than clobbering it with zero — same contract as the existing
// *bool handling.
func setFieldFromEnv(field reflect.Value, v string) {
	switch field.Kind() {
	case reflect.String:
		field.SetString(v)
	case reflect.Bool:
		field.SetBool(strings.ToLower(v) == "true")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, err := strconv.ParseInt(v, 10, field.Type().Bits()); err == nil {
			field.SetInt(n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, err := strconv.ParseUint(v, 10, field.Type().Bits()); err == nil {
			field.SetUint(n)
		}
	case reflect.Float32, reflect.Float64:
		if f, err := strconv.ParseFloat(v, field.Type().Bits()); err == nil {
			field.SetFloat(f)
		}
	case reflect.Pointer:
		setPtrFieldFromEnv(field, v)
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			field.Set(reflect.ValueOf(parseCSVTags(v)))
		}
	}
}

// setPtrFieldFromEnv handles `*T` for every primitive `T` supported by
// setFieldFromEnv. The dereferenced type drives parsing; a failed parse
// leaves the pointer at its prior value (typically nil).
func setPtrFieldFromEnv(field reflect.Value, v string) {
	elem := reflect.New(field.Type().Elem()).Elem()
	switch elem.Kind() {
	case reflect.String:
		elem.SetString(v)
	case reflect.Bool:
		b := parseBoolPtr(v)
		if b == nil {
			return
		}
		field.Set(reflect.ValueOf(b))
		return
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(v, 10, elem.Type().Bits())
		if err != nil {
			return
		}
		elem.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(v, 10, elem.Type().Bits())
		if err != nil {
			return
		}
		elem.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(v, elem.Type().Bits())
		if err != nil {
			return
		}
		elem.SetFloat(f)
	default:
		return
	}
	ptr := reflect.New(field.Type().Elem())
	ptr.Elem().Set(elem)
	field.Set(ptr)
}

// formatFieldForEnv renders field back to the string the subprocess env
// should see. An empty string signals "don't emit this env var" — the
// caller skips the line. Types not listed here silently produce "".
func formatFieldForEnv(field reflect.Value) string {
	switch field.Kind() {
	case reflect.String:
		return field.String()
	case reflect.Bool:
		if field.Bool() {
			return "true"
		}
		return ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(field.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(field.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(field.Float(), 'g', -1, field.Type().Bits())
	case reflect.Pointer:
		if field.IsNil() {
			return ""
		}
		return formatFieldForEnv(field.Elem())
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
// Both true and false bool entries are emitted so that explicit "do not
// install this default package" overrides (e.g. DD_INSTALLER_DEFAULT_PKG_INSTALL_X=false)
// propagate intact to child processes.
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
			val = strconv.FormatBool(iter.Value().Bool())
		default:
			continue
		}
		suffix = strings.ToUpper(strings.ReplaceAll(suffix, "-", "_"))
		*out = append(*out, envVar+"_"+suffix+"="+val)
	}
}

// Extension-registry env-var scheme. The suffix is the package name and
// extension name separated by a double underscore; single underscores in
// either half map to `-` on read (and back to `_` on write), matching the
// convention of the other prefix scanners.
//
//	DD_INSTALLER_REGISTRY_EXT_URL_<PKG>__<EXT>=<url>
//	DD_INSTALLER_REGISTRY_EXT_AUTH_<PKG>__<EXT>=<auth>
//	DD_INSTALLER_REGISTRY_EXT_USERNAME_<PKG>__<EXT>=<user>
//	DD_INSTALLER_REGISTRY_EXT_PASSWORD_<PKG>__<EXT>=<password>
const (
	extRegPrefixURL      = "DD_INSTALLER_REGISTRY_EXT_URL_"
	extRegPrefixAuth     = "DD_INSTALLER_REGISTRY_EXT_AUTH_"
	extRegPrefixUsername = "DD_INSTALLER_REGISTRY_EXT_USERNAME_"
	extRegPrefixPassword = "DD_INSTALLER_REGISTRY_EXT_PASSWORD_"
)

// parseExtensionRegistryEnv scans os.Environ() for extension-registry env
// vars and populates env.ExtensionRegistryOverrides. Env vars that don't
// carry the `<PKG>__<EXT>` split are skipped silently.
func parseExtensionRegistryEnv(env *Env) {
	setters := []struct {
		prefix string
		set    func(*ExtensionRegistryOverride, string)
	}{
		{extRegPrefixURL, func(o *ExtensionRegistryOverride, v string) { o.URL = v }},
		{extRegPrefixAuth, func(o *ExtensionRegistryOverride, v string) { o.Auth = v }},
		{extRegPrefixUsername, func(o *ExtensionRegistryOverride, v string) { o.Username = v }},
		{extRegPrefixPassword, func(o *ExtensionRegistryOverride, v string) { o.Password = v }},
	}
	for _, kv := range os.Environ() {
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		for _, s := range setters {
			suffix, found := strings.CutPrefix(key, s.prefix)
			if !found {
				continue
			}
			pkg, ext, ok := strings.Cut(suffix, "__")
			if !ok || pkg == "" || ext == "" {
				break
			}
			pkg = strings.ToLower(strings.ReplaceAll(pkg, "_", "-"))
			ext = strings.ToLower(strings.ReplaceAll(ext, "_", "-"))
			if env.ExtensionRegistryOverrides == nil {
				env.ExtensionRegistryOverrides = map[string]map[string]ExtensionRegistryOverride{}
			}
			if env.ExtensionRegistryOverrides[pkg] == nil {
				env.ExtensionRegistryOverrides[pkg] = map[string]ExtensionRegistryOverride{}
			}
			o := env.ExtensionRegistryOverrides[pkg][ext]
			s.set(&o, val)
			env.ExtensionRegistryOverrides[pkg][ext] = o
			break
		}
	}
}

// emitExtensionRegistryEnv emits one env var per non-empty field of each
// (pkg, ext) entry, symmetric with parseExtensionRegistryEnv.
func emitExtensionRegistryEnv(out *[]string, overrides map[string]map[string]ExtensionRegistryOverride) {
	for pkg, extMap := range overrides {
		pkgKey := strings.ToUpper(strings.ReplaceAll(pkg, "-", "_"))
		for ext, o := range extMap {
			extKey := strings.ToUpper(strings.ReplaceAll(ext, "-", "_"))
			suffix := pkgKey + "__" + extKey
			if o.URL != "" {
				*out = append(*out, extRegPrefixURL+suffix+"="+o.URL)
			}
			if o.Auth != "" {
				*out = append(*out, extRegPrefixAuth+suffix+"="+o.Auth)
			}
			if o.Username != "" {
				*out = append(*out, extRegPrefixUsername+suffix+"="+o.Username)
			}
			if o.Password != "" {
				*out = append(*out, extRegPrefixPassword+suffix+"="+o.Password)
			}
		}
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
