// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// TestDefaultCatalog_DetectorTeardownContract is the structural guard that
// every catalog detector either implements observerdef.SeriesRemover or is
// explicitly listed in statelessDetectorAllowlist. Without this, a new
// detector with per-series state can be added to the catalog and silently
// leak memory in production: storage eviction will free the series, but the
// detector's per-series map will never shrink.
func TestDefaultCatalog_DetectorTeardownContract(t *testing.T) {
	require.NoError(t, defaultCatalog().validateDetectorTeardownContract(),
		"every catalog detector must implement SeriesRemover or be added to statelessDetectorAllowlist with a justification comment")
}

// TestValidateDetectorTeardownContract_FlagsBareDetector confirms the
// validator rejects a Detector that doesn't implement SeriesRemover and isn't
// allowlisted — i.e. the check actually fails when it should.
func TestValidateDetectorTeardownContract_FlagsBareDetector(t *testing.T) {
	cat := &componentCatalog{
		entries: []componentEntry{
			{
				name:           "bare-detector",
				kind:           componentDetector,
				factory:        func(any) any { return &bareDetectorForValidator{} },
				defaultEnabled: true,
			},
		},
	}
	err := cat.validateDetectorTeardownContract()
	require.Error(t, err)
	var contractErr *detectorTeardownContractError
	require.True(t, errors.As(err, &contractErr), "error must be detectorTeardownContractError")
	require.Equal(t, "bare-detector", contractErr.name)
}

// TestValidateDetectorTeardownContract_AllowlistEscape confirms an allowlisted
// detector is permitted to skip SeriesRemover. Useful for genuinely stateless
// detectors (none in the catalog today; this exercises the escape hatch).
func TestValidateDetectorTeardownContract_AllowlistEscape(t *testing.T) {
	statelessDetectorAllowlist["explicitly-stateless-test"] = struct{}{}
	t.Cleanup(func() { delete(statelessDetectorAllowlist, "explicitly-stateless-test") })

	cat := &componentCatalog{
		entries: []componentEntry{
			{
				name:           "explicitly-stateless-test",
				kind:           componentDetector,
				factory:        func(any) any { return &bareDetectorForValidator{} },
				defaultEnabled: true,
			},
		},
	}
	require.NoError(t, cat.validateDetectorTeardownContract())
}

// bareDetectorForValidator is a minimal observerdef.Detector that
// intentionally does NOT implement SeriesRemover — used to drive the
// negative cases of validateDetectorTeardownContract.
type bareDetectorForValidator struct{}

func (*bareDetectorForValidator) Name() string { return "bare-detector" }
func (*bareDetectorForValidator) Detect(_ observerdef.StorageReader, _ int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}
