// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreambootstrap holds the global-config-builder helpers the configstreamconsumer
// component delegates to. Lives outside comp/ because the pkgconfigusage depguard blocks
// pkg/config/setup imports from comp/.
package configstreambootstrap

import (
	"math"

	"google.golang.org/protobuf/types/known/structpb"

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

// InitGlobalConfig wraps pkgconfigsetup.InitConfigObjects (forbidden from comp/).
func InitGlobalConfig(cliConfigPath, defaultConfPath string) {
	pkgconfigsetup.InitConfigObjects(cliConfigPath, defaultConfPath)
}

// ReadBaseSettings returns defaults+env from the global builder. Callers layer YAML on top.
func ReadBaseSettings() Settings {
	b := pkgconfigsetup.GlobalConfigBuilder()
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

// DisableLocalEnvLayer drops the env layer (nodetreemodel only) so local DD_* vars
// can't override streamed values.
func DisableLocalEnvLayer(clientName string) {
	type envVarClearer interface{ ClearEnvVars() }
	if clearer, ok := pkgconfigsetup.GlobalConfigBuilder().(envVarClearer); ok {
		clearer.ClearEnvVars()
		pkglog.Infof("configstreamconsumer[%s]: local env-var layer disabled", clientName)
		return
	}
	pkglog.Warnf("configstreamconsumer[%s]: config impl does not support disabling the env-var layer; subprocess env vars may override streamed values", clientName)
}

// AuthTokenFilepath resolves the auth-token path via pkg/api/security's fallback rules.
func AuthTokenFilepath() string {
	return pkgtoken.GetAuthTokenFilepath(pkgconfigsetup.GlobalConfigBuilder())
}

// IPCCertFilepath returns the configured ipc_cert_file_path.
func IPCCertFilepath() string {
	return pkgconfigsetup.GlobalConfigBuilder().GetString("ipc_cert_file_path")
}

// ApplySetting writes one streamed setting to the global builder, preserving the source.
func ApplySetting(key string, value *structpb.Value, source string) {
	pkgconfigsetup.GlobalConfigBuilder().Set(key, pbValueToGo(value), pkgconfigmodel.Source(source))
}

// pbValueToGo preserves integer types that structpb widens to float64.
// Bounded to |x| <= 2^53 — beyond that float64 loses integer precision.
func pbValueToGo(v *structpb.Value) any {
	if v == nil {
		return nil
	}
	result := v.AsInterface()
	if f, ok := result.(float64); ok {
		const maxExactInt = 1 << 53
		if !math.IsNaN(f) && !math.IsInf(f, 0) && f >= -maxExactInt && f <= maxExactInt && f == math.Trunc(f) {
			return int64(f)
		}
	}
	return result
}
