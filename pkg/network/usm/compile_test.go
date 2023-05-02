// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package usm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestUSMCompile(t *testing.T) {
	if !rtcHTTPSupported(t) {
		t.Skip("USM Runtime compilation not supported on this kernel version")
	}
	cfg := config.New()
	cfg.BPFDebug = true
	_, err := getRuntimeCompiledUSM(cfg)
	require.NoError(t, err)
}

func rtcHTTPSupported(t *testing.T) bool {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	return currKernelVersion >= http.MinimumKernelVersion
}
