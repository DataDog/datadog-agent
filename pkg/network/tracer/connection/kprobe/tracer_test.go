// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kprobe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestTracerFallback(t *testing.T) {
	if err := isCORETracerSupported(); err == errCORETracerNotSupported {
		t.Skip("CORE tracer not supported on this platform")
	} else {
		require.NoError(t, err)
	}

	prevRCLoader := rcTracerLoader
	prevPrebuiltLoader := prebuiltTracerLoader
	prevCORELoader := coreTracerLoader
	prevRunner := tracerOffsetGuesserRunner
	t.Cleanup(func() {
		tracerOffsetGuesserRunner = prevRunner
		rcTracerLoader = prevRCLoader
		prebuiltTracerLoader = prevPrebuiltLoader
		coreTracerLoader = prevCORELoader
	})

	testTracerFallbackNoErr(t)
	testTracerFallbackRCErr(t)
	testTracerFallbackCOREErr(t)
	testTracerFallbackCOREAndRCErr(t)
}

func testTracerFallbackNoErr(t *testing.T) {
	tests := []struct {
		enableCORE            bool
		allowRCFallback       bool
		enableRC              bool
		allowPrebuiltFallback bool

		tracerType TracerType
		err        error
	}{
		{false, false, false, false, TracerTypePrebuilt, nil},
		{false, false, false, true, TracerTypePrebuilt, nil},
		{false, false, true, false, TracerTypeRuntimeCompiled, nil},
		{false, false, true, true, TracerTypeRuntimeCompiled, nil},
		{false, true, false, false, TracerTypePrebuilt, nil},
		{false, true, false, true, TracerTypePrebuilt, nil},
		{false, true, true, false, TracerTypeRuntimeCompiled, nil},
		{false, true, true, true, TracerTypeRuntimeCompiled, nil},
		{true, false, false, false, TracerTypeCORE, nil},
		{true, false, false, true, TracerTypeCORE, nil},
		{true, false, true, false, TracerTypeCORE, nil},
		{true, false, true, true, TracerTypeCORE, nil},
		{true, true, false, false, TracerTypeCORE, nil},
		{true, true, false, true, TracerTypeCORE, nil},
		{true, true, true, false, TracerTypeCORE, nil},
		{true, true, true, true, TracerTypeCORE, nil},
	}

	runFallbackTests(t, "no error", false, false, tests)
}

func testTracerFallbackCOREErr(t *testing.T) {
	tests := []struct {
		enableCORE            bool
		allowRCFallback       bool
		enableRC              bool
		allowPrebuiltFallback bool

		tracerType TracerType
		err        error
	}{
		{false, false, false, false, TracerTypePrebuilt, nil},
		{false, false, false, true, TracerTypePrebuilt, nil},
		{false, false, true, false, TracerTypeRuntimeCompiled, nil},
		{false, false, true, true, TracerTypeRuntimeCompiled, nil},
		{false, true, false, false, TracerTypePrebuilt, nil},
		{false, true, false, true, TracerTypePrebuilt, nil},
		{false, true, true, false, TracerTypeRuntimeCompiled, nil},
		{false, true, true, true, TracerTypeRuntimeCompiled, nil},
		{true, false, false, false, TracerTypeCORE, assert.AnError},
		{true, false, false, true, TracerTypePrebuilt, nil},
		{true, false, true, false, TracerTypeCORE, assert.AnError},
		{true, false, true, true, TracerTypePrebuilt, nil},
		{true, true, false, false, TracerTypeCORE, assert.AnError},
		{true, true, false, true, TracerTypePrebuilt, nil},
		{true, true, true, false, TracerTypeRuntimeCompiled, nil},
		{true, true, true, true, TracerTypeRuntimeCompiled, nil},
	}

	runFallbackTests(t, "CORE error", true, false, tests)
}

func testTracerFallbackRCErr(t *testing.T) {
	tests := []struct {
		enableCORE            bool
		allowRCFallback       bool
		enableRC              bool
		allowPrebuiltFallback bool

		tracerType TracerType
		err        error
	}{
		{false, false, false, false, TracerTypePrebuilt, nil},
		{false, false, false, true, TracerTypePrebuilt, nil},
		{false, false, true, false, TracerTypeRuntimeCompiled, assert.AnError},
		{false, false, true, true, TracerTypePrebuilt, nil},
		{false, true, false, false, TracerTypePrebuilt, nil},
		{false, true, false, true, TracerTypePrebuilt, nil},
		{false, true, true, false, TracerTypeRuntimeCompiled, assert.AnError},
		{false, true, true, true, TracerTypePrebuilt, nil},
		{true, false, false, false, TracerTypeCORE, nil},
		{true, false, false, true, TracerTypeCORE, nil},
		{true, false, true, false, TracerTypeCORE, nil},
		{true, false, true, true, TracerTypeCORE, nil},
		{true, true, false, false, TracerTypeCORE, nil},
		{true, true, false, true, TracerTypeCORE, nil},
		{true, true, true, false, TracerTypeCORE, nil},
		{true, true, true, true, TracerTypeCORE, nil},
	}

	runFallbackTests(t, "RC error", false, true, tests)
}

func testTracerFallbackCOREAndRCErr(t *testing.T) {
	tests := []struct {
		enableCORE            bool
		allowRCFallback       bool
		enableRC              bool
		allowPrebuiltFallback bool

		tracerType TracerType
		err        error
	}{
		{false, false, false, false, TracerTypePrebuilt, nil},
		{false, false, false, true, TracerTypePrebuilt, nil},
		{false, false, true, false, TracerTypeRuntimeCompiled, assert.AnError},
		{false, false, true, true, TracerTypePrebuilt, nil},
		{false, true, false, false, TracerTypePrebuilt, nil},
		{false, true, false, true, TracerTypePrebuilt, nil},
		{false, true, true, false, TracerTypeRuntimeCompiled, assert.AnError},
		{false, true, true, true, TracerTypePrebuilt, nil},
		{true, false, false, false, TracerTypeCORE, assert.AnError},
		{true, false, false, true, TracerTypePrebuilt, nil},
		{true, false, true, false, TracerTypeCORE, assert.AnError},
		{true, false, true, true, TracerTypePrebuilt, nil},
		{true, true, false, false, TracerTypeCORE, assert.AnError},
		{true, true, false, true, TracerTypePrebuilt, nil},
		{true, true, true, false, TracerTypeRuntimeCompiled, assert.AnError},
		{true, true, true, true, TracerTypePrebuilt, nil},
	}

	runFallbackTests(t, "CORE and RC error", true, true, tests)
}

func loaderFunc(closeFn func(), err error) func(_ *config.Config, _ manager.Options, _ ddebpf.EventHandler, _ ddebpf.EventHandler) (*manager.Manager, func(), error) {
	return func(_ *config.Config, _ manager.Options, _ ddebpf.EventHandler, _ ddebpf.EventHandler) (*manager.Manager, func(), error) {
		return nil, closeFn, err
	}
}

func prebuiltLoaderFunc(closeFn func(), err error) func(_ *config.Config, _ manager.Options, _ ddebpf.EventHandler) (*manager.Manager, func(), error) {
	return func(_ *config.Config, _ manager.Options, _ ddebpf.EventHandler) (*manager.Manager, func(), error) {
		return nil, closeFn, err
	}
}

func runFallbackTests(t *testing.T, desc string, coreErr, rcErr bool, tests []struct {
	enableCORE            bool
	allowRCFallback       bool
	enableRC              bool
	allowPrebuiltFallback bool

	tracerType TracerType
	err        error
}) {
	expectedCloseFn := func() {}
	rcTracerLoader = loaderFunc(expectedCloseFn, nil)
	coreTracerLoader = loaderFunc(expectedCloseFn, nil)
	prebuiltTracerLoader = prebuiltLoaderFunc(expectedCloseFn, nil)
	if rcErr {
		rcTracerLoader = loaderFunc(nil, assert.AnError)
	}
	if coreErr {
		coreTracerLoader = loaderFunc(nil, assert.AnError)
	}

	offsetGuessingRun := 0
	tracerOffsetGuesserRunner = func(*config.Config) ([]manager.ConstantEditor, error) {
		offsetGuessingRun++
		return nil, nil
	}

	cfg := config.New()
	for _, te := range tests {
		t.Run(desc, func(t *testing.T) {
			cfg.EnableCORE = te.enableCORE
			cfg.AllowRuntimeCompiledFallback = te.allowRCFallback
			cfg.EnableRuntimeCompiler = te.enableRC
			cfg.AllowPrebuiltFallback = te.allowPrebuiltFallback

			prevOffsetGuessingRun := offsetGuessingRun
			_, closeFn, tracerType, err := LoadTracer(cfg, manager.Options{}, nil, nil)
			if te.err == nil {
				assert.NoError(t, err, "%+v", te)
			} else {
				assert.Error(t, err, "%+v", te)
			}

			if te.err != nil {
				assert.Nil(t, closeFn, "%+v", te)
			}

			assert.Equal(t, te.tracerType, tracerType, "%+v", te)

			if te.err == nil {
				if te.tracerType == TracerTypePrebuilt {
					// check if offset guesser was called
					assert.Equal(t, prevOffsetGuessingRun+1, offsetGuessingRun, "%+v: offset guesser was not called", te)
				} else {
					assert.Equal(t, prevOffsetGuessingRun, offsetGuessingRun, "%+v: offset guesser was called", te)
				}
			}
		})
	}

}

func TestCORETracerSupported(t *testing.T) {
	prevCORELoader := coreTracerLoader
	prevPrebuiltLoader := prebuiltTracerLoader
	t.Cleanup(func() {
		coreTracerLoader = prevCORELoader
		prebuiltTracerLoader = prevPrebuiltLoader
	})

	coreCalled := false
	coreTracerLoader = func(*config.Config, manager.Options, ddebpf.EventHandler, ddebpf.EventHandler) (*manager.Manager, func(), error) {
		coreCalled = true
		return nil, nil, nil
	}
	prebuiltCalled := false
	prebuiltTracerLoader = func(*config.Config, manager.Options, ddebpf.EventHandler) (*manager.Manager, func(), error) {
		prebuiltCalled = true
		return nil, nil, nil
	}

	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	platform, err := kernel.Platform()
	require.NoError(t, err)

	cfg := config.New()
	cfg.EnableCORE = true
	cfg.AllowRuntimeCompiledFallback = false
	_, _, _, err = LoadTracer(cfg, manager.Options{}, nil, nil)
	assert.False(t, prebuiltCalled)
	if kv < kernel.VersionCode(4, 4, 128) && platform != "centos" && platform != "redhat" {
		assert.False(t, coreCalled)
		assert.ErrorIs(t, err, errCORETracerNotSupported)
	} else {
		assert.True(t, coreCalled)
		assert.NoError(t, err)
	}

	coreCalled = false
	prebuiltCalled = false
	cfg.AllowRuntimeCompiledFallback = true
	_, _, _, err = LoadTracer(cfg, manager.Options{}, nil, nil)
	assert.NoError(t, err)
	if kv < kernel.VersionCode(4, 4, 128) && platform != "centos" && platform != "redhat" {
		assert.False(t, coreCalled)
		assert.True(t, prebuiltCalled)
	} else {
		assert.True(t, coreCalled)
		assert.False(t, prebuiltCalled)
	}
}

func TestDefaultKprobeMaxActiveSet(t *testing.T) {
	prevLoader := tracerLoaderFromAsset
	tracerLoaderFromAsset = func(_ bytecode.AssetReader, _, _ bool, _ *config.Config, mgrOpts manager.Options, _ ddebpf.EventHandler, _ ddebpf.EventHandler) (*manager.Manager, func(), error) {
		assert.Equal(t, mgrOpts.DefaultKProbeMaxActive, 128)
		return nil, nil, nil
	}
	t.Cleanup(func() { tracerLoaderFromAsset = prevLoader })

	t.Run("CO-RE", func(t *testing.T) {
		cfg := config.New()
		cfg.EnableCORE = true
		cfg.AllowRuntimeCompiledFallback = false
		_, _, _, err := LoadTracer(cfg, manager.Options{DefaultKProbeMaxActive: 128}, nil, nil)
		require.NoError(t, err)
	})

	t.Run("prebuilt", func(t *testing.T) {
		cfg := config.New()
		cfg.EnableCORE = false
		cfg.AllowRuntimeCompiledFallback = false
		_, _, _, err := LoadTracer(cfg, manager.Options{DefaultKProbeMaxActive: 128}, nil, nil)
		require.NoError(t, err)
	})

	t.Run("runtime_compiled", func(t *testing.T) {
		cfg := config.New()
		cfg.EnableCORE = false
		cfg.AllowRuntimeCompiledFallback = true
		_, _, _, err := LoadTracer(cfg, manager.Options{DefaultKProbeMaxActive: 128}, nil, nil)
		require.NoError(t, err)
	})
}
