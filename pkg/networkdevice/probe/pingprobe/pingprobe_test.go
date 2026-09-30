// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pingprobe

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/failure"
)

func testOptions() Options {
	return Options{Count: 1, Interval: 10 * time.Millisecond, Timeout: 100 * time.Millisecond}
}

func TestRunReportsAnUnresolvableHostAsAReading(t *testing.T) {
	r := Run("ndm-probe-test.invalid", testOptions())

	require.NotNil(t, r)
	assert.False(t, r.Success)
	assert.NotEmpty(t, r.FailureReason)
	assert.Contains(t, r.Error, "ndm-probe-test.invalid")
	assert.Zero(t, r.RTT)
}

func TestRunNeverReportsSuccessWithoutAnAnswer(t *testing.T) {
	r := Run("ndm-probe-test.invalid", testOptions())

	require.NotNil(t, r)
	assert.NotEqual(t, failure.None, r.FailureReason)
}

func TestFingerprintIsStableAndSensitiveToEveryOption(t *testing.T) {
	base := testOptions()
	assert.Equal(t, base.Fingerprint(), testOptions().Fingerprint())

	for name, opts := range map[string]Options{
		"count":    {Count: 2, Interval: base.Interval, Timeout: base.Timeout},
		"interval": {Count: base.Count, Interval: time.Second, Timeout: base.Timeout},
		"timeout":  {Count: base.Count, Interval: base.Interval, Timeout: time.Second},
		"raw":      {Count: base.Count, Interval: base.Interval, Timeout: base.Timeout, UseRawSocket: true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, base.Fingerprint(), opts.Fingerprint())
		})
	}
}

func TestFingerprintIsPrefixedWithTheProbeKind(t *testing.T) {
	assert.True(t, strings.HasPrefix(testOptions().Fingerprint(), "ping:"))
}

func TestDetectReportsAReasonWhenItIsNotAvailable(t *testing.T) {
	c := Detect()

	if c.Available {
		assert.Empty(t, c.Reason)
		return
	}
	assert.NotEmpty(t, c.Reason)
}
