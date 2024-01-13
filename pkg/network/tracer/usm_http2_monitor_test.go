// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	usmhttp2 "github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type usmHTTP2Suite struct {
	suite.Suite
	isTLS bool
}

func TestHTTP2Scenarios(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < usmhttp2.MinimumKernelVersion {
		t.Skipf("HTTP2 monitoring can not run on kernel before %v", usmhttp2.MinimumKernelVersion)
	}

	for _, tc := range []struct {
		name  string
		isTLS bool
	}{
		{
			name:  "without TLS",
			isTLS: false,
		},
		{
			name:  "with TLS",
			isTLS: true,
		},
	} {
		ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, tc.name, func(t *testing.T) {
			if tc.isTLS {
				if !goTLSSupported() {
					t.Skip("GoTLS not supported for this setup")
				}

				if skipFedora(t) {
					// GoTLS fails consistently in CI on Fedora 36,37
					t.Skip("TestHTTP2Scenarios fails on this OS consistently")
				}
			}

			suite.Run(t, &usmHTTP2Suite{isTLS: tc.isTLS})
		})
	}
}
