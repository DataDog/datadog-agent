// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func BenchmarkLoadOrStoreReset(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, telemetryimpl.MockModule())
	// benchmark with the internal telemetry enabled
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)

	sInterner := newLegacyStringInterner(4, 1, stringInternerTelemetry)

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
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newLegacyStringInterner(3, 1, stringInternerTelemetry)

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
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newLegacyStringInterner(4, 1, stringInternerTelemetry)

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
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newLegacyStringInterner(4, 1, stringInternerTelemetry)

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

func TestUniqueInternLoadOrStoreValue(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	assert := assert.New(t)
	sInterner := newUniqueStringInterner(0, 1, stringInternerTelemetry)

	foo := []byte("foo")
	bar := []byte("bar")
	far := []byte("far")
	boo := []byte("boo")

	// Test that the correct value is returned
	v := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v)
	v = sInterner.LoadOrStore(bar)
	assert.Equal("bar", v)
	v = sInterner.LoadOrStore(far)
	assert.Equal("far", v)
	v = sInterner.LoadOrStore(boo)
	assert.Equal("boo", v)
}

func TestUniqueInternLoadOrStorePointerEquality(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newUniqueStringInterner(0, 1, stringInternerTelemetry)

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

	// Test pointer equality for same strings
	v1 := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v1)

	v2 := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v2)
	assert.Equal(&v1, &v2, "same string should return same pointer (interned)")

	// Test different strings have different pointers
	v3 := sInterner.LoadOrStore(bar)
	assert.Equal("bar", v3)
	assert.NotEqual(&v1, &v3, "different strings should have different pointers")

	// Test that same string returns same pointer even after other strings
	v4 := sInterner.LoadOrStore(bar)
	assert.Equal("bar", v4)
	assert.Equal(&v3, &v4, "same string should return same pointer after other operations")

	v5 := sInterner.LoadOrStore(boo)
	assert.Equal("boo", v5)
	assert.NotEqual(&v1, &v5, "different strings should have different pointers")
	assert.NotEqual(&v3, &v5, "different strings should have different pointers")

	// Test same string one more time after multiple other strings
	v6 := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v6)
	assert.Equal(&v1, &v6, "same string should still return same pointer")
}
