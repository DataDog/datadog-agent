// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"testing"

	"github.com/stretchr/testify/require"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

func TestOffsetGuess(t *testing.T) {
	cfg := testConfig()
	offsetBuf, err := netebpf.ReadOffsetBPFModule(cfg.BPFDir, cfg.BPFDebug)
	require.NoError(t, err, "could not read offset bpf module")
	t.Cleanup(func() { offsetBuf.Close() })
	_, err = runOffsetGuessing(cfg, offsetBuf)
	require.NoError(t, err)
}
