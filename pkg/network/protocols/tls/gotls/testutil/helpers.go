// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"github.com/DataDog/gopsutil/host"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

// isFedora returns true if the current OS is Fedora.
// go-tls does not work correctly on Fedora 35, 36, 37 and 38.
func isFedora() bool {
	info, err := host.Info()
	if err != nil {
		return false
	}
	return info.Platform == "fedora" && (info.PlatformVersion == "35" || info.PlatformVersion == "36" || info.PlatformVersion == "37" || info.PlatformVersion == "38")
}

// GoTLSSupported returns true if GO-TLS monitoring is supported on the current OS.
func GoTLSSupported(cfg *config.Config) bool {
	return http.TLSSupported(cfg) && (cfg.EnableRuntimeCompiler || cfg.EnableCORE) && !isFedora()
}
