// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
)

// GoTLSSupported returns true if GO-TLS monitoring is supported on the current OS.
func GoTLSSupported(cfg *config.Config) bool {
	return usmconfig.TLSSupported(cfg) && (cfg.EnableRuntimeCompiler || cfg.EnableCORE)
}
