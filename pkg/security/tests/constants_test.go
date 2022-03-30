// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests && linux_bpf
// +build functionaltests,linux_bpf

package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
)

func TestOctogonConstants(t *testing.T) {
	kv, err := kernel.NewKernelVersion()
	if err != nil {
		t.Fatal(err)
	}

	dir, err := os.MkdirTemp("", "test-octogon-constants")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0o711); err != nil {
		t.Fatal(err)
	}

	config, err := genTestConfig(dir, testOpts{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("rc-vs-fallback", func(t *testing.T) {
		checkKernelCompatibility(t, "SLES and Oracle kernels", func(kv *kernel.Version) bool {
			return kv.IsSLESKernel() || kv.IsOracleUEKKernel()
		})

		fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)
		rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&config.Config, nil)

		assertConstantsEqual(t, rcFetcher, fallbackFetcher, kv)
	})

	t.Run("btfhub-vs-rc", func(t *testing.T) {
		checkKernelCompatibility(t, "SLES and Oracle kernels", func(kv *kernel.Version) bool {
			return kv.IsSLESKernel() || kv.IsOracleUEKKernel()
		})

		btfhubFetcher, err := constantfetch.NewBTFHubConstantFetcher(kv)
		if err != nil {
			t.Skipf("btfhub constant fetcher is not available: %v", err)
		}
		if !btfhubFetcher.HasConstantsInStore() {
			t.Skip("btfhub has no constant for this OS")
		}

		rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&config.Config, nil)

		assertConstantsEqual(t, rcFetcher, btfhubFetcher, kv)
	})

	t.Run("btf-vs-fallback", func(t *testing.T) {
		btfFetcher, err := constantfetch.NewBTFConstantFetcherFromCurrentKernel()
		if err != nil {
			t.Skipf("btf constant fetcher is not available: %v", err)
		}

		fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)

		assertConstantsEqual(t, btfFetcher, fallbackFetcher, kv)
	})

	t.Run("guesser-vs-rc", func(t *testing.T) {
		checkKernelCompatibility(t, "SLES and Oracle kernels", func(kv *kernel.Version) bool {
			return kv.IsSLESKernel() || kv.IsOracleUEKKernel()
		})

		rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&config.Config, nil)
		ogFetcher := constantfetch.NewOffsetGuesserFetcher(config)

		assertConstantContains(t, rcFetcher, ogFetcher, kv)
	})
}

func getFighterConstants(champion, challenger constantfetch.ConstantFetcher, kv *kernel.Version) (map[string]uint64, map[string]uint64, error) {
	championConstants, err := getOffsetConstantsFromFetcher(champion, kv)
	if err != nil {
		return nil, nil, err
	}

	challengerConstants, err := getOffsetConstantsFromFetcher(challenger, kv)
	if err != nil {
		return nil, nil, err
	}

	return championConstants, challengerConstants, nil
}

func assertConstantsEqual(t *testing.T, champion, challenger constantfetch.ConstantFetcher, kv *kernel.Version) {
	t.Helper()
	championConstants, challengerConstants, err := getFighterConstants(champion, challenger, kv)
	if err != nil {
		t.Error(err)
	}

	if !assert.Equal(t, championConstants, challengerConstants) {
		t.Logf("comparison between `%s`(-) and `%s`(+). kernel version: %v", champion.String(), challenger.String(), kv)
	}
}

func assertConstantContains(t *testing.T, champion, challenger constantfetch.ConstantFetcher, kv *kernel.Version) {
	t.Helper()
	championConstants, challengerConstants, err := getFighterConstants(champion, challenger, kv)
	if err != nil {
		t.Error(err)
	}

	for k, v := range challengerConstants {
		if v == constantfetch.ErrorSentinel {
			continue
		}

		if expected, ok := championConstants[k]; !ok || v != expected {
			t.Errorf("expected %s:%d not found", k, v)
		}
	}
}

func getOffsetConstantsFromFetcher(cf constantfetch.ConstantFetcher, kv *kernel.Version) (map[string]uint64, error) {
	probe.AppendProbeRequestsToFetcher(cf, kv)
	return cf.FinishAndGetResults()
}
