// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestTracerCompile(t *testing.T) {
	cfg := config.New()
	cfg.BPFDebug = true
	_, err := getRuntimeCompiledTracer(cfg)
	require.NoError(t, err)
}
