// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package controlconfig defines the effective Agent configuration handed from
// the Go config loader to the split Private Action Runner control plane.
package controlconfig

import (
	"context"
	"path/filepath"

	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
)

const (
	// SchemaVersion is incremented when the Go-to-Rust JSON contract changes.
	SchemaVersion = 1

	defaultIPCCertFileName = "ipc_cert.pem"
)

// ProxyConfig is the Agent's effective proxy configuration after environment
// precedence, secret resolution, FIPS handling, and config transforms.
type ProxyConfig struct {
	HTTP    string   `json:"http,omitempty"`
	HTTPS   string   `json:"https,omitempty"`
	NoProxy []string `json:"no_proxy,omitempty"`
}

// EffectiveConfig contains only settings needed by par-control. Values come
// from the fully initialized Agent config component rather than raw YAML, so
// secret-backend values and post-load transforms are preserved.
type EffectiveConfig struct {
	SchemaVersion            int               `json:"schema_version"`
	MainEndpoint             string            `json:"main_endpoint"`
	TaskConcurrency          int               `json:"task_concurrency"`
	ExecutorSocket           string            `json:"executor_socket"`
	ProcmgrSocket            string            `json:"procmgr_socket"`
	ExecutorProcessName      string            `json:"executor_process_name"`
	IdleTimeoutSeconds       int               `json:"idle_timeout_seconds"`
	HeartbeatIntervalSeconds int               `json:"heartbeat_interval_seconds"`
	OPMSExtraHeaders         map[string]string `json:"opms_extra_headers"`
	Proxy                    ProxyConfig       `json:"proxy"`
	NoProxyNonexactMatch     bool              `json:"no_proxy_nonexact_match"`
	SkipSSLValidation        bool              `json:"skip_ssl_validation"`
	MinTLSVersion            string            `json:"min_tls_version"`
	IPCCertFile              string            `json:"ipc_cert_file"`
	URN                      string            `json:"urn,omitempty"`
	PrivateKey               string            `json:"private_key,omitempty"`
}

// Resolve snapshots the settings par-control needs from the Agent's effective
// configuration. Callers must pass a config that has completed LoadDatadog.
func Resolve(cfg configmodel.Reader) EffectiveConfig {
	var proxy ProxyConfig
	if effectiveProxy := cfg.GetProxies(); effectiveProxy != nil {
		proxy = ProxyConfig{
			HTTP:    effectiveProxy.HTTP,
			HTTPS:   effectiveProxy.HTTPS,
			NoProxy: effectiveProxy.NoProxy,
		}
	}

	return EffectiveConfig{
		SchemaVersion:            SchemaVersion,
		MainEndpoint:             configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"),
		TaskConcurrency:          cfg.GetInt("private_action_runner.task_concurrency"),
		ExecutorSocket:           cfg.GetString("private_action_runner.executor.socket_path"),
		ProcmgrSocket:            cfg.GetString("private_action_runner.procmgr_socket_path"),
		ExecutorProcessName:      cfg.GetString("private_action_runner.executor_process_name"),
		IdleTimeoutSeconds:       cfg.GetInt("private_action_runner.idle_timeout_seconds"),
		HeartbeatIntervalSeconds: cfg.GetInt("private_action_runner.heartbeat_interval_seconds"),
		OPMSExtraHeaders:         cfg.GetStringMapString("private_action_runner.opms_extra_headers"),
		Proxy:                    proxy,
		NoProxyNonexactMatch:     cfg.GetBool("no_proxy_nonexact_match"),
		SkipSSLValidation:        cfg.GetBool("skip_ssl_validation"),
		MinTLSVersion:            cfg.GetString("min_tls_version"),
		IPCCertFile: resolveSiblingPath(
			cfg.GetString("ipc_cert_file_path"),
			cfg.GetString("auth_token_file_path"),
			cfg.ConfigFileUsed(),
			defaultIPCCertFileName,
		),
		URN:        cfg.GetString("private_action_runner.urn"),
		PrivateKey: cfg.GetString("private_action_runner.private_key"),
	}
}

// ResolveWithIdentity applies the monolith's persisted-identity selection on
// top of Resolve. In particular, an identity tied to an old hostname is not
// handed to Rust; that causes the normal self-enrollment path to replace it.
func ResolveWithIdentity(ctx context.Context, cfg configmodel.Reader, agentIdentifier *enrollment.AgentIdentifier) (EffectiveConfig, error) {
	effective := Resolve(cfg)
	persisted, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return EffectiveConfig{}, err
	}
	if enrollment.ShouldReenroll(agentIdentifier, persisted) {
		persisted = nil
	}
	if persisted != nil {
		effective.URN = persisted.URN
		effective.PrivateKey = persisted.PrivateKey
	}
	return effective, nil
}

func resolveSiblingPath(explicit, authTokenPath, configFile, name string) string {
	if explicit != "" {
		return explicit
	}
	if authTokenPath != "" {
		return filepath.Join(filepath.Dir(authTokenPath), name)
	}
	return filepath.Join(filepath.Dir(configFile), name)
}
