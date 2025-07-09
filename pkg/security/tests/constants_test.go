// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests && linux_bpf

// Package tests holds tests related files
package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
)

var BTFHubVsFallbackPossiblyMissingConstants = []string{
	constantfetch.OffsetNameNFConnStructCTNet,
	constantfetch.OffsetNameTaskStructPID,
	constantfetch.OffsetNameTaskStructPIDLink,
	constantfetch.OffsetNameDeviceStructNdNet,
	constantfetch.OffsetNameSockStructSKProtocol,
}

var BTFVsFallbackPossiblyMissingConstants = []string{
	constantfetch.OffsetNameIoKiocbStructCtx,
	constantfetch.OffsetNameTaskStructPID,
	constantfetch.OffsetNameTaskStructPIDLink,
	constantfetch.OffsetNameDeviceStructNdNet,
	constantfetch.OffsetNameSockStructSKProtocol,
}

func TestOctogonConstants(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := constantfetch.NewBTFConstantFetcherFromCurrentKernel(); err == nil {
		t.Skipf("this kernel has BTF data available, skipping octogon")
	}

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

	t.Run("btfhub-vs-fallback", func(t *testing.T) {
		btfhubFetcher, err := constantfetch.NewBTFHubConstantFetcher(kv)
		if err != nil {
			t.Skipf("btfhub constant fetcher is not available: %v", err)
		}

		fallbackFetcher := constantfetch.NewFallbackConstantFetcher(kv)

		assertConstantsEqual(t, btfhubFetcher, fallbackFetcher, kv, BTFHubVsFallbackPossiblyMissingConstants)
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

func getOffsetConstantsFromFetcher(cf constantfetch.ConstantFetcher, kv *kernel.Version) (map[string]uint64, error) {
	probe.AppendProbeRequestsToFetcher(cf, kv)
	return cf.FinishAndGetResults()
}
