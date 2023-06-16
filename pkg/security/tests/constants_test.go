// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests && linux_bpf

package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
)

var BTFHubVsRcPossiblyMissingConstants = []string{
	constantfetch.OffsetNameNFConnStructCTNet,
	constantfetch.OffsetNameIoKiocbStructCtx,
}

var RCVsFallbackPossiblyMissingConstants = []string{
	constantfetch.OffsetNameIoKiocbStructCtx,
	constantfetch.OffsetNameTaskStructPID,
	constantfetch.OffsetNameTaskStructPIDLink,
}

var BTFHubVsFallbackPossiblyMissingConstants = []string{
	constantfetch.OffsetNameNFConnStructCTNet,
	constantfetch.OffsetNameTaskStructPID,
	constantfetch.OffsetNameTaskStructPIDLink,
}

var BTFVsFallbackPossiblyMissingConstants = []string{
	constantfetch.OffsetNameTaskStructPID,
	constantfetch.OffsetNameTaskStructPIDLink,
}

func TestOctogonConstants(t *testing.T) {
	if err := initLogger(); err != nil {
		t.Fatal(err)
	}

	kv, err := kernel.NewKernelVersion()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	if err := os.Chmod(dir, 0o711); err != nil {
		t.Fatal(err)
	}

	_, secconfig, err := genTestConfigs(dir, testOpts{}, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("rc-vs-fallback", func(t *testing.T) {
		checkKernelCompatibility(t, "SLES kernels", func(kv *kernel.Version) bool {
			return kv.IsSLESKernel()
		})

		fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)
		rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&secconfig.Probe.Config, nil)

		assertConstantsEqual(t, rcFetcher, fallbackFetcher, kv, RCVsFallbackPossiblyMissingConstants)
	})

	t.Run("btfhub-vs-rc", func(t *testing.T) {
		checkKernelCompatibility(t, "SLES kernels", func(kv *kernel.Version) bool {
			return kv.IsSLESKernel()
		})

		btfhubFetcher, err := constantfetch.NewBTFHubConstantFetcher(kv)
		if err != nil {
			t.Skipf("btfhub constant fetcher is not available: %v", err)
		}
		if !btfhubFetcher.HasConstantsInStore() {
			t.Skip("btfhub has no constant for this OS")
		}

		rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&secconfig.Probe.Config, nil)

		assertConstantsEqual(t, rcFetcher, btfhubFetcher, kv, BTFHubVsRcPossiblyMissingConstants)
	})

	t.Run("btfhub-vs-fallback", func(t *testing.T) {
		btfhubFetcher, err := constantfetch.NewBTFHubConstantFetcher(kv)
		if err != nil {
			t.Skipf("btfhub constant fetcher is not available: %v", err)
		}
		if !btfhubFetcher.HasConstantsInStore() {
			t.Skip("btfhub has no constant for this OS")
		}

		fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)

		assertConstantsEqual(t, btfhubFetcher, fallbackFetcher, kv, BTFHubVsFallbackPossiblyMissingConstants)
	})

	t.Run("btf-vs-fallback", func(t *testing.T) {
		btfFetcher, err := constantfetch.NewBTFConstantFetcherFromCurrentKernel()
		if err != nil {
			t.Skipf("btf constant fetcher is not available: %v", err)
		}

		fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)

		assertConstantsEqual(t, btfFetcher, fallbackFetcher, kv, BTFVsFallbackPossiblyMissingConstants)
	})

	t.Run("guesser-vs-rc", func(t *testing.T) {
		checkKernelCompatibility(t, "SLES kernels", func(kv *kernel.Version) bool {
			return kv.IsSLESKernel()
		})

		rcFetcher := constantfetch.NewRuntimeCompilationConstantFetcher(&secconfig.Probe.Config, nil)
		ogFetcher := constantfetch.NewOffsetGuesserFetcher(secconfig.Probe, kv)

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

func assertConstantsEqual(t *testing.T, champion, challenger constantfetch.ConstantFetcher, kv *kernel.Version, ignoreMissing []string) {
	t.Helper()
	championConstants, challengerConstants, err := getFighterConstants(champion, challenger, kv)
	if err != nil {
		t.Error(err)
	}

	for _, possiblyMissingConstant := range ignoreMissing {
		championValue, championPresent := championConstants[possiblyMissingConstant]
		challengerValue, challengerPresent := challengerConstants[possiblyMissingConstant]

		// if the constant is not present in the champion or the challenger
		// we let the `assert.Equal` do its job and trigger an error or not
		if !championPresent || !challengerPresent {
			continue
		}

		if championValue != constantfetch.ErrorSentinel && challengerValue == constantfetch.ErrorSentinel {
			delete(championConstants, possiblyMissingConstant)
			delete(challengerConstants, possiblyMissingConstant)
		}

		if championValue == constantfetch.ErrorSentinel && challengerValue != constantfetch.ErrorSentinel {
			delete(championConstants, possiblyMissingConstant)
			delete(challengerConstants, possiblyMissingConstant)
		}
	}

	if !assert.Equal(t, championConstants, challengerConstants) {
		t.Logf("comparison between `%s`(-) and `%s`(+)", champion.String(), challenger.String())
		t.Logf("kernel version: %v", kv)
	}
}

func assertConstantContains(t *testing.T, champion, challenger constantfetch.ConstantFetcher, kv *kernel.Version) {
	t.Helper()
	championConstants, challengerConstants, err := getFighterConstants(champion, challenger, kv)
	if err != nil {
		t.Error(err)
	}

	if len(challengerConstants) == 0 {
		t.Errorf("challenger %s has no constant\n", challenger)
	}

	for k, v := range challengerConstants {
		if v == constantfetch.ErrorSentinel {
			continue
		}

		expected, ok := championConstants[k]
		if !ok {
			t.Errorf("champion (`%s`) does not contain the expected constant `%s`", champion.String(), k)
		} else if v != expected {
			t.Errorf("difference between fighters for `%s`: `%s`:%d and `%s`:%d", k, champion.String(), expected, challenger.String(), v)
		}
	}

	if t.Failed() {
		t.Logf("kernel version: %v", kv)
	}
}

func getOffsetConstantsFromFetcher(cf constantfetch.ConstantFetcher, kv *kernel.Version) (map[string]uint64, error) {
	probe.AppendProbeRequestsToFetcher(cf, kv)
	return cf.FinishAndGetResults()
}
