// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
)

// Default configuration values
const (
	DefaultSymbolQueryInterval  = 5 * time.Second
	DefaultUploadSymbolsDryRun  = false
	DefaultUploadSymbolsHTTP2   = false
	DefaultUploadSymbols        = true
	DefaultUploadDynamicSymbols = false
	DefaultUploadGoPCLnTab      = true
)

type SymbolUploaderOptions struct {
	// Enabled defines whether the agent should upload debug symbols to the backend.
	Enabled bool `mapstructure:"enabled"`
	// UploadDynamicSymbols defines whether the agent should upload dynamic symbols to the backend.
	UploadDynamicSymbols bool `mapstructure:"upload_dynamic_symbols"`
	// UploadGoPCLnTab defines whether the agent should upload GoPCLnTab section for Go binaries to the backend.
	UploadGoPCLnTab bool `mapstructure:"upload_go_pcln_tab"`
	// UseHTTP2 defines whether the agent should use HTTP/2 when uploading symbols.
	UseHTTP2 bool `mapstructure:"use_http2"`
	// SymbolQueryInterval defines the interval at which the agent should query the backend for symbols. A value of 0 disables batching.
	SymbolQueryInterval time.Duration `mapstructure:"symbol_query_interval"`
	// DryRun defines whether the agent should upload debug symbols to the backend in dry-run mode.
	DryRun bool `mapstructure:"dry_run"`
	// Sites to upload symbols to.
	SymbolEndpoints []SymbolEndpoint `mapstructure:"symbol_endpoints"`
}

type SymbolUploaderConfig struct {
	// Options defines the options for the symbol uploader.
	SymbolUploaderOptions `mapstructure:",squash"`

	// DisableDebugSectionCompression defines whether the uploader should disable debug section compression whatever objcopy supports.
	// This is only used for testing purposes.
	DisableDebugSectionCompression bool `mapstructure:"disable_debug_section_compression"`
	// Version is the version of the profiler.
	Version string `mapstructure:"-"`
	// Name is the name of the profiler.
	Name string `mapstructure:"-"`
}

func DefaultSymbolUploaderConfig() SymbolUploaderConfig {
	return SymbolUploaderConfig{
		SymbolUploaderOptions: SymbolUploaderOptions{
			Enabled:              DefaultUploadSymbols,
			UploadDynamicSymbols: DefaultUploadDynamicSymbols,
			UploadGoPCLnTab:      DefaultUploadGoPCLnTab,
			UseHTTP2:             DefaultUploadSymbolsHTTP2,
			SymbolQueryInterval:  DefaultSymbolQueryInterval,
			DryRun:               DefaultUploadSymbolsDryRun,
		},
		Name:    version.ProfilerName,
		Version: version.ProfilerVersion,
	}
}
