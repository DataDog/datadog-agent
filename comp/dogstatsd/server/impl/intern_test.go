// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func BenchmarkLoadOrStoreReset(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	// benchmark with the internal telemetry enabled
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)

	sInterner := newStringInterner(4, 1, stringInternerTelemetry)

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
	sInterner := newStringInterner(3, 1, stringInternerTelemetry)

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
	sInterner := newStringInterner(4, 1, stringInternerTelemetry)

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

func TestInternLoadOrStoreBoundedEviction(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(8, 1, stringInternerTelemetry)

	// First sightings go through the recent segment.
	sInterner.LoadOrStore([]byte("foo"))
	assert.Equal(1, sInterner.len())
	sInterner.LoadOrStore([]byte("bar"))
	assert.Equal(2, sInterner.len())

	// A second sighting promotes a key to protected, where it survives recent
	// churn instead of being lost in a whole-cache reset.
	sInterner.LoadOrStore([]byte("foo"))
	assert.Contains(sInterner.protected, "foo")
	assert.Equal(2, sInterner.len())

	sInterner.LoadOrStore([]byte("boo"))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("val"))
	assert.LessOrEqual(sInterner.len(), 8)
	assert.Contains(sInterner.protected, "foo")
}

func TestInternLoadOrStoreProtectedEviction(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(4, 1, stringInternerTelemetry)

	for _, value := range []string{"a", "a", "b", "b", "c", "c", "d", "d", "e", "e"} {
		sInterner.LoadOrStore([]byte(value))
	}

	assert.LessOrEqual(sInterner.len(), 4)
	assert.LessOrEqual(len(sInterner.protected), sInterner.protectedMax)
	assert.LessOrEqual(len(sInterner.recent), sInterner.recentMax)
}
