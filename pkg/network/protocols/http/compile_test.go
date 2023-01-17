// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestHttpCompile(t *testing.T) {
	if !rtcHTTPSupported(t) {
		t.Skip("HTTP Runtime compilation not supported on this kernel version")
	}
	cfg := config.New()
	cfg.BPFDebug = true
	_, err := getRuntimeCompiledHTTP(cfg)
	require.NoError(t, err)
}

func rtcHTTPSupported(t *testing.T) bool {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	return currKernelVersion >= MinimumKernelVersion
}
