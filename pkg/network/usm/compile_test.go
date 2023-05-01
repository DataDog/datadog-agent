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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestHttpCompile(t *testing.T) {
	if !rtcHTTPSupported() {
		t.Skip("HTTP Runtime compilation not supported on this kernel version")
	}
	cfg := config.New()
	cfg.BPFDebug = true
	_, err := getRuntimeCompiledHTTP(cfg)
	require.NoError(t, err)
}

func rtcHTTPSupported() bool {
	currKernelVersion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. http monitoring disabled.")
		return false
	}
	return currKernelVersion >= http.MinimumKernelVersion
}
