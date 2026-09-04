// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreambootstrap holds the global-config-builder helpers the configstreamconsumer
// component delegates to. Lives outside comp/ because the pkgconfigusage depguard blocks
// pkg/config/setup imports from comp/.
package configstreambootstrap

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"

	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Settings is the bounded set of values resolved from env+YAML before dial.
// Everything else comes from the streamed snapshot.
type Settings struct {
	AuthTokenFilePath  string
	IPCCertFilePath    string
	CmdHost            string
	CmdPort            int
	VSockAddr          string
	RARRegistryEnabled bool
}

// ReadBaseSettings returns the bootstrap settings from the global config (defaults + env layer).
func ReadBaseSettings() Settings {
	b := pkgconfigsetup.Datadog()
	return Settings{
		AuthTokenFilePath:  b.GetString("auth_token_file_path"),
		IPCCertFilePath:    b.GetString("ipc_cert_file_path"),
		CmdHost:            b.GetString("cmd_host"),
		CmdPort:            b.GetInt("cmd_port"),
		VSockAddr:          b.GetString("vsock_addr"),
		RARRegistryEnabled: b.GetBool("remote_agent.registry.enabled"),
	}
}

// SeedGlobalBuilder writes bootstrap values to the global builder. configFile is recorded
// as ConfigFileUsed so pkg/api/security[/cert] fallback paths resolve next to datadog.yaml.
func SeedGlobalBuilder(s Settings, configFile string) {
	b := pkgconfigsetup.GlobalConfigBuilder()
	if configFile != "" {
		b.SetConfigFile(configFile)
	}
	if s.AuthTokenFilePath != "" {
		b.Set("auth_token_file_path", s.AuthTokenFilePath, pkgconfigmodel.SourceFile)
	}
	if s.IPCCertFilePath != "" {
		b.Set("ipc_cert_file_path", s.IPCCertFilePath, pkgconfigmodel.SourceFile)
	}
	b.Set("cmd_host", s.CmdHost, pkgconfigmodel.SourceFile)
	b.Set("cmd_port", s.CmdPort, pkgconfigmodel.SourceFile)
	if s.VSockAddr != "" {
		b.Set("vsock_addr", s.VSockAddr, pkgconfigmodel.SourceFile)
	}
	b.Set("remote_agent.registry.enabled", s.RARRegistryEnabled, pkgconfigmodel.SourceFile)

	// Resolve fallback paths (next-to-datadog.yaml or next-to-auth_token) and persist them
	// so subsequent GetString calls return concrete paths instead of empty strings.
	pkgtoken.PersistAuthTokenFilepath(b)
	cert.PersistCertFilepath(b)
}

// envOverride is a setting the local env layer was deciding the value of before the wipe.
type envOverride struct {
	key    string
	envVar string
	value  any
}

var (
	// DisableLocalEnvLayer runs on the main goroutine at startup and the report runs on the
	// consumer's stream goroutine; the ordering is not enforced by this package's API, so lock.
	envOverridesMu        sync.Mutex
	capturedEnvOverrides  []envOverride
	lastEnvOverrideReport []string
)

// DisableLocalEnvLayer drops the env layer (nodetreemodel only) so local DD_* vars
// can't override streamed values. Viper-backed configs cannot clear env vars.
func DisableLocalEnvLayer(clientName string) {
	b := pkgconfigsetup.Datadog()
	type envVarClearer interface{ ClearEnvVars() }
	clearer, ok := b.(envVarClearer)
	if !ok {
		return
	}
	type configEnvVarLister interface{ ConfigEnvVars() map[string][]string }
	if lister, ok := b.(configEnvVarLister); ok {
		envOverridesMu.Lock()
		capturedEnvOverrides = captureEnvOverrides(b, lister.ConfigEnvVars())
		envOverridesMu.Unlock()
	}
	clearer.ClearEnvVars()
	pkglog.Infof("configstreamconsumer[%s]: local env-var layer disabled", clientName)
}

// captureEnvOverrides records the settings the env layer is actually deciding. A key whose env var
// is set but loses to a higher-precedence source was not being overridden by the env, so it is skipped.
func captureEnvOverrides(cfg pkgconfigmodel.Reader, configEnvVars map[string][]string) []envOverride {
	captured := make([]envOverride, 0, len(configEnvVars))
	for key, envVars := range configEnvVars {
		if cfg.GetSource(key) != pkgconfigmodel.SourceEnvVar {
			continue
		}
		if name := winningEnvVar(envVars); name != "" {
			captured = append(captured, envOverride{key: key, envVar: name, value: cfg.Get(key)})
		}
	}
	slices.SortFunc(captured, func(a, b envOverride) int { return strings.Compare(a.key, b.key) })
	return captured
}

// winningEnvVar mirrors nodetreemodel.buildEnvVars: the first var that is set and non-empty wins.
func winningEnvVar(envVars []string) string {
	for _, name := range envVars {
		if value, isSet := os.LookupEnv(name); isSet && value != "" {
			return name
		}
	}
	return ""
}

// ReportDroppedEnvOverrides warns about settings the stream did not reproduce. It must run after
// the first snapshot and after any post-snapshot remapping, and consumes the captured state so
// that later value changes, which are ordinary operation, never warn.
func ReportDroppedEnvOverrides(clientName string) {
	envOverridesMu.Lock()
	captured := capturedEnvOverrides
	capturedEnvOverrides = nil
	envOverridesMu.Unlock()
	if len(captured) == 0 {
		return
	}

	dropped := diffEnvOverrides(pkgconfigsetup.Datadog(), captured)

	envOverridesMu.Lock()
	lastEnvOverrideReport = dropped
	envOverridesMu.Unlock()

	if len(dropped) == 0 {
		return
	}
	pkglog.Warnf("configstreamconsumer[%s]: these settings were set by DD_* env vars on this process and the core Agent streamed a different value, so the local value is lost; set them on the core Agent instead: %s",
		clientName, strings.Join(dropped, ", "))
}

// diffEnvOverrides names the captured settings whose current value differs from the pre-wipe one.
// Names only, never values: several of these settings are credentials, and "differs" is the signal.
func diffEnvOverrides(cfg pkgconfigmodel.Reader, captured []envOverride) []string {
	var dropped []string
	for _, o := range captured {
		// Values can be maps or slices, so == would panic.
		if reflect.DeepEqual(cfg.Get(o.key), o.value) {
			continue
		}
		dropped = append(dropped, fmt.Sprintf("%s (%s)", o.key, o.envVar))
	}
	return dropped
}

// AuthTokenFilepath resolves the auth-token path via pkg/api/security's fallback rules.
func AuthTokenFilepath() string {
	return pkgtoken.GetAuthTokenFilepath(pkgconfigsetup.Datadog())
}

// IPCCertFilepath returns the configured ipc_cert_file_path.
func IPCCertFilepath() string {
	return pkgconfigsetup.Datadog().GetString("ipc_cert_file_path")
}

// Config returns the global config builder the streamed settings are written to.
func Config() pkgconfigmodel.Config {
	return pkgconfigsetup.Datadog()
}
