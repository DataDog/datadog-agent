// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var maxBackoffDuration = time.Duration(maxBackoffTime) * time.Second

func TestMaxErrors(t *testing.T) {
	previousBackoffDuration := time.Duration(0) * time.Second
	attempts := 0

	for i := 1; ; i++ {
		backoffDuration := GetBackoffDuration(i)

		if i > 1000 {
			assert.Truef(t, i < 1000, "Too many iterations")
		} else if backoffDuration == previousBackoffDuration {
			attempts = i - 1
			break
		}

		previousBackoffDuration = backoffDuration
	}

	assert.Equal(t, maxErrors, attempts)
}

func TestBlock(t *testing.T) {
	e := newBlockedEndpoints()

	e.close("test")
	now := time.Now()

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, now.Before(e.errorPerEndpoint["test"].until))
}

func TestMaxBlock(t *testing.T) {
	e := newBlockedEndpoints()
	e.close("test")
	e.errorPerEndpoint["test"].nbError = 1000000

	e.close("test")
	now := time.Now()

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.Equal(t, maxErrors, e.errorPerEndpoint["test"].nbError)
	assert.True(t, now.Add(maxBackoffDuration).After(e.errorPerEndpoint["test"].until) ||
		now.Add(maxBackoffDuration).Equal(e.errorPerEndpoint["test"].until))
}

func TestUnblock(t *testing.T) {
	e := newBlockedEndpoints()

	e.close("test")
	require.Contains(t, e.errorPerEndpoint, "test")
	e.close("test")
	e.close("test")
	e.close("test")
	e.close("test")

	e.recover("test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == int(math.Max(0, float64(5-recoveryInterval))))
}

func TestMaxUnblock(t *testing.T) {
	e := newBlockedEndpoints()

	e.close("test")
	e.recover("test")
	e.recover("test")
	now := time.Now()

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == 0)
	assert.True(t, now.After(e.errorPerEndpoint["test"].until) || now.Equal(e.errorPerEndpoint["test"].until))
}

func TestUnblockUnknown(t *testing.T) {
	e := newBlockedEndpoints()

	e.recover("test")
	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == 0)
}

func TestIsBlock(t *testing.T) {
	e := newBlockedEndpoints()

	assert.False(t, e.isBlock("test"))

	e.close("test")
	assert.True(t, e.isBlock("test"))

	e.recover("test")
	assert.False(t, e.isBlock("test"))
}

func TestIsBlockTiming(t *testing.T) {
	e := newBlockedEndpoints()

	// setting an old close
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(-time.Duration(30 * time.Second))}
	assert.False(t, e.isBlock("test"))

	// setting an new close
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(time.Duration(30 * time.Second))}
	assert.True(t, e.isBlock("test"))
}

func TestIsblockUnknown(t *testing.T) {
	e := newBlockedEndpoints()

	assert.False(t, e.isBlock("test"))
}
