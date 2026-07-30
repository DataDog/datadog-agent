// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogtelextensionimpl provides the implementation of the dogtelextension component.
package dogtelextensionimpl

import (
	"fmt"
)

// Config defines the configuration for the dogtelextension
type Config struct {
	// EnableMetadataCollection toggles metadata collection. Pointer so that
	// nil ("not set") is distinguishable from an explicit false, allowing
	// NewConfigComponent to leave the DD agent default (true) intact when
	// the field is absent.
	EnableMetadataCollection *bool `mapstructure:"enable_metadata_collection"`
	// MetadataInterval is the metadata collection interval, in seconds.
	// 0 uses the agent default (1800).
	MetadataInterval int `mapstructure:"metadata_interval"`

	// EnableTaggerServer enables the tagger gRPC server.
	EnableTaggerServer bool `mapstructure:"enable_tagger_server"`
	// TaggerServerPort is the port the tagger server binds to. 0 auto-assigns.
	TaggerServerPort int `mapstructure:"tagger_server_port"`
	// TaggerServerAddr is the address the tagger server binds to. Default: localhost.
	TaggerServerAddr string `mapstructure:"tagger_server_addr"`
	// TaggerMaxMessageSize is the maximum gRPC message size in bytes. Default: 4MB.
	TaggerMaxMessageSize int `mapstructure:"tagger_max_message_size"`
	// TaggerMaxConcurrentSync is the maximum number of concurrent tag stream
	// subscribers. Default: 5.
	TaggerMaxConcurrentSync int `mapstructure:"tagger_max_concurrent_sync"`

	// StandaloneMode controls whether the extension runs in standalone mode,
	// enabling secrets resolution and other agent features. Typically set via
	// the DD_OTEL_STANDALONE environment variable.
	StandaloneMode bool `mapstructure:"standalone_mode"`

	// Hostname overrides the agent hostname used in standalone mode.
	// Maps to the "hostname" DD agent config key. When empty the agent
	// resolves the hostname automatically.
	Hostname string `mapstructure:"hostname"`

	// SecretBackendCommand is the path to the secret resolver binary
	// (standalone mode). Maps to the "secret_backend_command" DD agent
	// config key so ENC[] handles in the OTel config can be resolved
	// without a separate datadog.yaml file.
	SecretBackendCommand string `mapstructure:"secret_backend_command"`
	// SecretBackendArguments are extra CLI arguments passed to the secret
	// resolver binary.
	SecretBackendArguments []string `mapstructure:"secret_backend_arguments"`
	// SecretBackendTimeout is the secret-resolver execution timeout, in
	// seconds. 0 uses the agent default (30s).
	SecretBackendTimeout int `mapstructure:"secret_backend_timeout"`
	// SecretBackendOutputMaxSize is the maximum secret-resolver output size,
	// in bytes. 0 uses the agent default.
	SecretBackendOutputMaxSize int `mapstructure:"secret_backend_output_max_size"`

	// KubernetesKubeletHost is the kubelet host used for K8s tag enrichment
	// in standalone mode (e.g. "status.hostIP" or an explicit IP). When the
	// otel-agent runs on a Kubernetes node these settings let the workloadmeta
	// kubelet collector and the local tagger reach the kubelet API without
	// needing a separate datadog.yaml.
	KubernetesKubeletHost string `mapstructure:"kubernetes_kubelet_host"`
	// KubeletTLSVerify controls TLS verification for the kubelet client.
	// nil keeps the DD agent default (true).
	KubeletTLSVerify *bool `mapstructure:"kubelet_tls_verify"`
	// KubernetesHTTPKubeletPort is the kubelet HTTP port. 0 uses the agent
	// default (10255).
	KubernetesHTTPKubeletPort int `mapstructure:"kubernetes_http_kubelet_port"`
	// KubernetesHTTPSKubeletPort is the kubelet HTTPS port. 0 uses the agent
	// default (10250).
	KubernetesHTTPSKubeletPort int `mapstructure:"kubernetes_https_kubelet_port"`
}

// Validate validates the configuration
func (cfg *Config) Validate() error {
	if cfg.TaggerServerPort < 0 || cfg.TaggerServerPort > 65535 {
		return fmt.Errorf("invalid tagger_server_port: %d (must be 0-65535)", cfg.TaggerServerPort)
	}

	if cfg.TaggerMaxMessageSize <= 0 {
		cfg.TaggerMaxMessageSize = 4 * 1024 * 1024 // 4MB default
	}

	if cfg.MetadataInterval < 0 {
		return fmt.Errorf("invalid metadata_interval: %d (must be >= 0)", cfg.MetadataInterval)
	}

	if cfg.TaggerMaxConcurrentSync <= 0 {
		cfg.TaggerMaxConcurrentSync = 5 // Default
	}

	return nil
}
