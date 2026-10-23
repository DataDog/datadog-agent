// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package oomkill

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// Config is the configuration of the OOM Kill probe
type Config struct {
	ebpf.Config

	// PersistMonitoring pins the probe to bpffs so that it keeps recording OOM kills after
	// system-probe exits without a graceful shutdown
	PersistMonitoring bool
}

// NewConfig creates a [Config] from the system-probe configuration
func NewConfig() *Config {
	return &Config{
		Config:            *ebpf.NewConfig(),
		PersistMonitoring: pkgconfigsetup.SystemProbe().GetBool("oom_kill.persist_monitoring"),
	}
}
