// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpftest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

func TestBuildModeConstants(t *testing.T) {
	TestBuildMode(t, Prebuilt, "", func(t *testing.T) {
		cfg := ebpf.NewConfig()
		assert.False(t, cfg.EnableRuntimeCompiler)
		assert.False(t, cfg.EnableCORE)
		assert.False(t, cfg.AllowPrebuiltFallback)
		assert.False(t, cfg.AllowRuntimeCompiledFallback)
		assert.Equal(t, "false", os.Getenv("NETWORK_TRACER_FENTRY_TESTS"))
	})
	TestBuildMode(t, RuntimeCompiled, "", func(t *testing.T) {
		cfg := ebpf.NewConfig()
		assert.True(t, cfg.EnableRuntimeCompiler)
		assert.False(t, cfg.EnableCORE)
		assert.False(t, cfg.AllowPrebuiltFallback)
		assert.False(t, cfg.AllowRuntimeCompiledFallback)
		assert.Equal(t, "false", os.Getenv("NETWORK_TRACER_FENTRY_TESTS"))
	})
	TestBuildMode(t, CORE, "", func(t *testing.T) {
		cfg := ebpf.NewConfig()
		assert.False(t, cfg.EnableRuntimeCompiler)
		assert.True(t, cfg.EnableCORE)
		assert.False(t, cfg.AllowPrebuiltFallback)
		assert.False(t, cfg.AllowRuntimeCompiledFallback)
		assert.Equal(t, "false", os.Getenv("NETWORK_TRACER_FENTRY_TESTS"))
	})
	TestBuildMode(t, Fentry, "", func(t *testing.T) {
		cfg := ebpf.NewConfig()
		assert.False(t, cfg.EnableRuntimeCompiler)
		assert.True(t, cfg.EnableCORE)
		assert.False(t, cfg.AllowPrebuiltFallback)
		assert.False(t, cfg.AllowRuntimeCompiledFallback)
		assert.Equal(t, "true", os.Getenv("NETWORK_TRACER_FENTRY_TESTS"))
	})
}
