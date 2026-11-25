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

func makeTagBytes(prefix string, n int) [][]byte {
	tags := make([][]byte, n)
	for i := range n {
		tags[i] = fmt.Appendf(nil, "%s%05d", prefix, i)
	}
	return tags
}

func BenchmarkLoadOrStoreSteadyState(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)
	sInterner := newStringInterner(4096, 1, stringInternerTelemetry)

	hot := makeTagBytes("service:my-service-", 2000)
	for _, tag := range hot {
		sInterner.LoadOrStore(tag)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		sInterner.LoadOrStore(hot[i%len(hot)])
	}
}

func BenchmarkLoadOrStoreSteadyStateNearCap(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)
	sInterner := newStringInterner(4096, 1, stringInternerTelemetry)

	hot := makeTagBytes("service:my-service-", 3500)
	for _, tag := range hot {
		sInterner.LoadOrStore(tag)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		sInterner.LoadOrStore(hot[i%len(hot)])
	}
}

func BenchmarkLoadOrStoreWorkingSetPlusChurn(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)
	sInterner := newStringInterner(4096, 1, stringInternerTelemetry)

	// 2000 hot tags (80% of lookups) + 16384 high-cardinality one-off tags (20% of lookups)
	hot := makeTagBytes("service:my-service-", 2000)
	cold := makeTagBytes("pod_name:app-deployment-7f8b9c-", 16384)
	for _, tag := range hot {
		sInterner.LoadOrStore(tag)
	}

	b.ReportAllocs()
	b.ResetTimer()
	hotIdx, coldIdx := 0, 0
	for i := range b.N {
		if i%5 == 0 {
			sInterner.LoadOrStore(cold[coldIdx%len(cold)])
			coldIdx++
		} else {
			sInterner.LoadOrStore(hot[hotIdx%len(hot)])
			hotIdx++
		}
	}
}

func BenchmarkLoadOrStorePureChurn(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)
	sInterner := newStringInterner(4096, 1, stringInternerTelemetry)

	cold := makeTagBytes("pod_name:app-deployment-7f8b9c-", 16384)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		sInterner.LoadOrStore(cold[i%len(cold)])
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

func TestInternLoadOrStoreReset(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(4, 1, stringInternerTelemetry)

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
