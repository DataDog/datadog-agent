// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	manager "github.com/DataDog/ebpf-manager"
)

func TestTracerFallback(t *testing.T) {
	prevRCLoader := rcTracerLoader
	prevPrebuiltLoader := prebuiltTracerLoader
	prevCORELoader := coreTracerLoader
	t.Cleanup(func() {
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
		enableCORE         bool
		enableCOREFallback bool
		enableRC           bool
		enableRCFallback   bool

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
		enableCORE         bool
		enableCOREFallback bool
		enableRC           bool
		enableRCFallback   bool

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
		{true, false, false, true, TracerTypeCORE, assert.AnError},
		{true, false, true, false, TracerTypeCORE, assert.AnError},
		{true, false, true, true, TracerTypeCORE, assert.AnError},
		{true, true, false, false, TracerTypePrebuilt, nil},
		{true, true, false, true, TracerTypePrebuilt, nil},
		{true, true, true, false, TracerTypeRuntimeCompiled, nil},
		{true, true, true, true, TracerTypeRuntimeCompiled, nil},
	}

	runFallbackTests(t, "CORE error", true, false, tests)
}

func testTracerFallbackRCErr(t *testing.T) {
	tests := []struct {
		enableCORE         bool
		enableCOREFallback bool
		enableRC           bool
		enableRCFallback   bool

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
		enableCORE         bool
		enableCOREFallback bool
		enableRC           bool
		enableRCFallback   bool

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
		{true, false, false, true, TracerTypeCORE, assert.AnError},
		{true, false, true, false, TracerTypeCORE, assert.AnError},
		{true, false, true, true, TracerTypeCORE, assert.AnError},
		{true, true, false, false, TracerTypePrebuilt, nil},
		{true, true, false, true, TracerTypePrebuilt, nil},
		{true, true, true, false, TracerTypeRuntimeCompiled, assert.AnError},
		{true, true, true, true, TracerTypePrebuilt, nil},
	}

	runFallbackTests(t, "CORE and RC error", true, true, tests)
}

func loaderFunc(closeFn func(), err error) func(_ *config.Config, _ *manager.Manager, _ manager.Options, _ *ddebpf.PerfHandler) (func(), error) {
	return func(_ *config.Config, _ *manager.Manager, _ manager.Options, _ *ddebpf.PerfHandler) (func(), error) {
		return closeFn, err
	}
}

func runFallbackTests(t *testing.T, desc string, coreErr, rcErr bool, tests []struct {
	enableCORE         bool
	enableCOREFallback bool
	enableRC           bool
	enableRCFallback   bool

	tracerType TracerType
	err        error
}) {
	expectedCloseFn := func() {}
	rcTracerLoader = loaderFunc(expectedCloseFn, nil)
	coreTracerLoader = loaderFunc(expectedCloseFn, nil)
	if rcErr {
		rcTracerLoader = loaderFunc(nil, assert.AnError)
	}
	if coreErr {
		coreTracerLoader = loaderFunc(nil, assert.AnError)
	}

	prebuiltTracerLoader = loaderFunc(expectedCloseFn, nil)

	cfg := config.New()
	for _, te := range tests {
		t.Run(desc, func(t *testing.T) {
			cfg.EnableCORE = te.enableCORE
			cfg.AllowRuntimeCompiledFallback = te.enableCOREFallback
			cfg.EnableRuntimeCompiler = te.enableRC
			cfg.AllowPrecompiledFallback = te.enableRCFallback

			closeFn, tracerType, err := LoadTracer(cfg, nil, manager.Options{}, nil)
			if te.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}

			if te.err != nil {
				assert.Nil(t, closeFn)
			}

			assert.Equal(t, te.tracerType, tracerType)
		})
	}

}
