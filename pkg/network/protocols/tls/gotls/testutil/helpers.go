// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	fedoraPlatform = "fedora"
)

var fedoraUnsupportedVersions = []string{"35", "36", "37", "38"}

// isFedora returns true if the current OS is Fedora.
// go-tls does not work correctly on Fedora 35, 36, 37 and 38.
func isFedora(t *testing.T) bool {
	platform, err := kernel.Platform()
	require.NoError(t, err)
	platformVersion, err := kernel.PlatformVersion()
	require.NoError(t, err)

	return platform == fedoraPlatform && slices.Contains(fedoraUnsupportedVersions, platformVersion)
}

// GoTLSSupported returns true if GO-TLS monitoring is supported on the current OS.
func GoTLSSupported(t *testing.T, cfg *config.Config) bool {
	return http.TLSSupported(cfg) && (cfg.EnableRuntimeCompiler || cfg.EnableCORE) && !isFedora(t)
}
