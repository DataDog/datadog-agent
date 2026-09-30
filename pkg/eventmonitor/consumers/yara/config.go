// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const configPrefix = "event_monitoring_config.yara."

// Config holds the YARA exec scanner configuration, read from event_monitoring_config.yara.*
// in system-probe.yaml
type Config struct {
	// Enabled turns the consumer on
	Enabled bool
	// RulesDir is the directory of rule files compiled at startup. Scanning stays off when empty.
	RulesDir string
	// ChanSize is the size of the consumer channel (exec events)
	ChanSize int
	// Workers is the number of concurrent scans
	Workers int
	// QueueSize is the number of pending scans before drop
	QueueSize int
	// MaxFileSize is the size in bytes above which files are skipped
	MaxFileSize int64
	// ScanTimeout is the per-file scan limit
	ScanTimeout time.Duration
	// IdentityCacheSize is the number of identity LRU entries
	IdentityCacheSize int
	// RecheckTTL is the time after which a known identity is re-hashed
	RecheckTTL time.Duration
}

// NewConfig reads the configuration from the system-probe config
func NewConfig() *Config {
	return newConfigFrom(pkgconfigsetup.SystemProbe())
}

func newConfigFrom(cfg model.Reader) *Config {
	return &Config{
		Enabled:           cfg.GetBool(configPrefix + "enabled"),
		RulesDir:          cfg.GetString(configPrefix + "rules_dir"),
		ChanSize:          cfg.GetInt(configPrefix + "chan_size"),
		Workers:           cfg.GetInt(configPrefix + "workers"),
		QueueSize:         cfg.GetInt(configPrefix + "queue_size"),
		MaxFileSize:       cfg.GetInt64(configPrefix + "max_file_size"),
		ScanTimeout:       cfg.GetDuration(configPrefix + "scan_timeout"),
		IdentityCacheSize: cfg.GetInt(configPrefix + "identity_cache_size"),
		RecheckTTL:        cfg.GetDuration(configPrefix + "recheck_ttl"),
	}
}
