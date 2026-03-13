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
	// Metadata collection settings.
	// EnableMetadataCollection is a pointer so that nil ("not set") is
	// distinguishable from an explicit false, allowing NewConfigComponent to
	// leave the DD agent default (true) intact when the field is absent.
	EnableMetadataCollection *bool `mapstructure:"enable_metadata_collection"`
	MetadataInterval         int   `mapstructure:"metadata_interval"` // seconds; 0 = use agent default (1800)

	// Tagger server settings
	EnableTaggerServer      bool   `mapstructure:"enable_tagger_server"`
	TaggerServerPort        int    `mapstructure:"tagger_server_port"`         // 0 = auto-assign
	TaggerServerAddr        string `mapstructure:"tagger_server_addr"`         // Default: localhost
	TaggerMaxMessageSize    int    `mapstructure:"tagger_max_message_size"`    // Default: 4MB
	TaggerMaxConcurrentSync int    `mapstructure:"tagger_max_concurrent_sync"` // Default: 5

	// Standalone mode (controls secrets and other features)
	// This is typically set via DD_OTEL_STANDALONE environment variable
	StandaloneMode bool `mapstructure:"standalone_mode"`

	// Hostname overrides the agent hostname used in standalone mode.
	// Maps to the "hostname" DD agent config key.
	// When empty the agent resolves the hostname automatically.
	Hostname string `mapstructure:"hostname"`

	// Secrets backend settings (standalone mode).
	// These map directly to the corresponding DD agent config keys so that
	// ENC[] handles in the OTel config can be resolved without a separate
	// datadog.yaml file.
	SecretBackendCommand       string   `mapstructure:"secret_backend_command"`         // path to the resolver binary
	SecretBackendArguments     []string `mapstructure:"secret_backend_arguments"`       // extra CLI arguments
	SecretBackendTimeout       int      `mapstructure:"secret_backend_timeout"`         // seconds; 0 = agent default (30s)
	SecretBackendOutputMaxSize int      `mapstructure:"secret_backend_output_max_size"` // bytes; 0 = agent default

	// Kubernetes / kubelet settings for K8s tag enrichment (standalone mode).
	// When the otel-agent runs on a Kubernetes node these allow the workloadmeta
	// kubelet collector and the local tagger to reach the kubelet API without
	// needing a separate datadog.yaml.
	KubernetesKubeletHost      string `mapstructure:"kubernetes_kubelet_host"`       // e.g. "status.hostIP" or an explicit IP
	KubeletTLSVerify           *bool  `mapstructure:"kubelet_tls_verify"`            // nil = keep DD agent default (true)
	KubernetesHTTPKubeletPort  int    `mapstructure:"kubernetes_http_kubelet_port"`  // 0 = agent default (10255)
	KubernetesHTTPSKubeletPort int    `mapstructure:"kubernetes_https_kubelet_port"` // 0 = agent default (10250)
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
