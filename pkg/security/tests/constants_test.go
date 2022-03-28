// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests && linux_bpf
// +build functionaltests,linux_bpf

package tests

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestFallbackConstants(t *testing.T) {
	checkKernelCompatibility(t, "SLES and Oracle kernels", func(kv *kernel.Version) bool {
		return kv.IsSLES12Kernel() || kv.IsSLES15Kernel() || kv.IsOracleUEKKernel()
	})

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	kv, err := test.probe.GetKernelVersion()
	if err != nil {
		t.Fatal(err)
	}
	config := test.config

	fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)
	rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&config.Config, nil)

	compareFetchers(t, rcFetcher, fallbackFetcher, kv)
}

func TestBTFHubConstants(t *testing.T) {
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	kv, err := test.probe.GetKernelVersion()
	if err != nil {
		t.Fatal(err)
	}
	config := test.config

	btfhubFetcher, err := constantfetch.NewBTFHubConstantFetcher(kv)
	if err != nil {
		t.Skipf("btfhub constant fetcher is not available: %v", err)
	}
	if !btfhubFetcher.HasConstantsInStore() {
		t.Skip("btfhub has no constant for this OS")
	}

	rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&config.Config, nil)

	compareFetchers(t, rcFetcher, btfhubFetcher, kv)
}

func TestBTFConstants(t *testing.T) {
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	kv, err := test.probe.GetKernelVersion()
	if err != nil {
		t.Fatal(err)
	}

	btfFetcher, err := constantfetch.NewBTFConstantFetcherFromCurrentKernel()
	if err != nil {
		t.Skipf("btf constant fetcher is not available: %v", err)
	}

	fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)

	compareFetchers(t, btfFetcher, fallbackFetcher, kv)
}

func compareFetchers(t *testing.T, expected, actual constantfetch.ConstantFetcher, kv *kernel.Version) {
	t.Helper()
	expectedConstants, err := GetOffsetConstantsFromFetcher(expected, kv)
	if err != nil {
		t.Error(err)
	}

	actualConstants, err := GetOffsetConstantsFromFetcher(actual, kv)
	if err != nil {
		t.Error(err)
	}

	if !assert.Equal(t, expectedConstants, actualConstants) {
		t.Logf("kernel version: %v", kv)
	}
}

func GetOffsetConstantsFromFetcher(cf constantfetch.ConstantFetcher, kv *kernel.Version) (map[string]uint64, error) {
	probe.AppendProbeRequestsToFetcher(cf, kv)
	return cf.FinishAndGetResults()
}
