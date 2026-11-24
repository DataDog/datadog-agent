// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func BenchmarkLoadOrStoreReset(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	// benchmark with the internal telemetry enabled
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)

	sInterner := newStringInterner(4, 1, stringInternerTelemetry, false)

	list := []string{}
	for i := 0; i < 512; i++ {
		list = append(list, fmt.Sprintf("testing.metric%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sInterner.LoadOrStore([]byte(list[i%len(list)]))
	}
}

func TestInternLoadOrStoreValue(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(3, 1, stringInternerTelemetry, false)

	foo := []byte("foo")
	bar := []byte("bar")
	far := []byte("far")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v)
	v = sInterner.LoadOrStore(bar)
	assert.Equal("bar", v)
	v = sInterner.LoadOrStore(far)
	assert.Equal("far", v)
	v = sInterner.LoadOrStore(boo)
	assert.Equal("boo", v)
}

func TestInternLoadOrStorePointer(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(4, 1, stringInternerTelemetry, false)

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v)
	v2 := sInterner.LoadOrStore(foo)
	assert.Equal(&v, &v2, "must point to the same address")
	v2 = sInterner.LoadOrStore(bar)
	assert.NotEqual(&v, &v2, "must point to a different address")
	v3 := sInterner.LoadOrStore(bar)
	assert.Equal(&v2, &v3, "must point to the same address")

	v4 := sInterner.LoadOrStore(boo)
	assert.NotEqual(&v, &v4, "must point to a different address")
	assert.NotEqual(&v2, &v4, "must point to a different address")
	assert.NotEqual(&v3, &v4, "must point to a different address")
}

func TestInternLoadOrStoreReset(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(4, 1, stringInternerTelemetry, false)

	// first test that the good value is returned.
	sInterner.LoadOrStore([]byte("foo"))
	assert.Equal(1, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("bar"))
	sInterner.LoadOrStore([]byte("bar"))
	assert.Equal(2, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("boo"))
	assert.Equal(3, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	assert.Equal(4, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(1, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(1, len(sInterner.strings))
}

func TestInternTwoGenerationTelemetry(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	require.True(t, ok)

	assertGauges := func(wantEntries, wantBytes float64) {
		t.Helper()
		entries, err := telemetryMock.GetGaugeMetric("dogstatsd", "string_interner_entries")
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, wantEntries, entries[0].Value())

		bytes, err := telemetryMock.GetGaugeMetric("dogstatsd", "string_interner_bytes")
		require.NoError(t, err)
		require.Len(t, bytes, 1)
		assert.Equal(t, wantBytes, bytes[0].Value())
	}

	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)
	sInterner := newStringInterner(2, 1, stringInternerTelemetry, true)

	sInterner.LoadOrStore([]byte("aaa"))
	sInterner.LoadOrStore([]byte("bb"))
	assertGauges(2, 5)

	// Rotate: prev={aaa, bb}, curr={cccc} -> 3 entries, 9 bytes
	sInterner.LoadOrStore([]byte("cccc"))
	assertGauges(3, 9)

	// Promote "aaa": curr={cccc, aaa}, prev={bb} -> still 3 entries, 9 bytes
	sInterner.LoadOrStore([]byte("aaa"))
	assertGauges(3, 9)

	// Next rotation evicts only "bb" (2 bytes): prev={cccc, aaa}, curr={d} -> 3 entries, 8 bytes
	sInterner.LoadOrStore([]byte("d"))
	assertGauges(3, 8)

	resets, err := telemetryMock.GetCountMetric("dogstatsd", "string_interner_resets")
	require.NoError(t, err)
	require.Len(t, resets, 1)
	assert.Equal(t, float64(2), resets[0].Value())
}

func TestInternTwoGenerationNonPositiveMaxSize(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(0, 1, stringInternerTelemetry, true)

	assert.False(t, sInterner.twoGeneration)
	assert.Equal(t, "foo", sInterner.LoadOrStore([]byte("foo")))
	assert.Equal(t, "bar", sInterner.LoadOrStore([]byte("bar")))
}
