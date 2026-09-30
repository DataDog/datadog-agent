// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func testPathtest() common.Pathtest {
	return common.Pathtest{
		Hostname: "db-01",
		Port:     5432,
		Protocol: payload.ProtocolTCP,
	}
}

func TestCorrelationKeyIsStable(t *testing.T) {
	first := makeCorrelationKey("web-01", testPathtest())
	require.Len(t, first, 32, "key must be 32 lowercase hex chars")
	assert.Regexp(t, "^[0-9a-f]{32}$", first)

	for range 100 {
		assert.Equal(t, first, makeCorrelationKey("web-01", testPathtest()))
	}
}

// The key is empty when the Agent cannot resolve its own hostname. That is a
// real state the UI handles by falling back to a filtered view, not an error.
func TestCorrelationKeyEmptyWithoutSourceHostname(t *testing.T) {
	assert.Empty(t, makeCorrelationKey("", testPathtest()))
}

// Every input must change the digest, or two different tests would share an
// identity and the pivot would deep-link to the wrong one.
func TestEveryInputAffectsTheKey(t *testing.T) {
	base := makeCorrelationKey("web-01", testPathtest())

	t.Run("source hostname", func(t *testing.T) {
		assert.NotEqual(t, base, makeCorrelationKey("web-02", testPathtest()))
	})

	t.Run("destination hostname", func(t *testing.T) {
		pt := testPathtest()
		pt.Hostname = "db-02"
		assert.NotEqual(t, base, makeCorrelationKey("web-01", pt))
	})

	t.Run("port", func(t *testing.T) {
		pt := testPathtest()
		pt.Port = 5433
		assert.NotEqual(t, base, makeCorrelationKey("web-01", pt))
	})

	t.Run("protocol", func(t *testing.T) {
		pt := testPathtest()
		pt.Protocol = payload.ProtocolUDP
		assert.NotEqual(t, base, makeCorrelationKey("web-01", pt))
	})
}

// Length-prefixing each field stops a boundary shift between two adjacent
// inputs from producing the same digest.
func TestFieldBoundariesCannotCollide(t *testing.T) {
	a := testPathtest()
	a.Hostname = "b-01"

	b := testPathtest()
	b.Hostname = "01"

	assert.NotEqual(t,
		makeCorrelationKey("web", a),
		makeCorrelationKey("web-b", b),
		"shifting characters across the hostname boundary must change the key")
}
