// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
)

func TestEbpfConntrackerLoadTriggersOffsetGuessing(t *testing.T) {
	offsetguess.TracerOffsets.Reset()

	cfg := testConfig()
	cfg.EnableRuntimeCompiler = false
	conntracker, err := NewEBPFConntracker(cfg)
	assert.NoError(t, err)
	require.NotNil(t, conntracker)
	t.Cleanup(conntracker.Close)

	offsets, err := offsetguess.TracerOffsets.Offsets(cfg)
	require.NoError(t, err)
	require.NotEmpty(t, offsets)
}
