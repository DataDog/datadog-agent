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

func compareFetchers(t *testing.T, a, b constantfetch.ConstantFetcher, kv *kernel.Version) {
	t.Helper()
	aConstants, err := probe.GetOffsetConstantsFromFetcher(a, kv)
	if err != nil {
		t.Error(err)
	}

	bConstants, err := probe.GetOffsetConstantsFromFetcher(b, kv)
	if err != nil {
		t.Error(err)
	}

	if !assert.Equal(t, aConstants, bConstants) {
		t.Logf("kernel version: %v", kv)
	}
}
