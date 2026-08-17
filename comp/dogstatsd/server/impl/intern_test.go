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

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func BenchmarkLoadOrStoreReset(b *testing.B) {
	telemetryComp := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	// benchmark with the internal telemetry enabled
	stringInternerTelemetry := newSiTelemetry(true, telemetryComp)

	sInterner := newStringInterner(tagset.NewTable(4), 1, stringInternerTelemetry)

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
	sInterner := newStringInterner(tagset.NewTable(3), 1, stringInternerTelemetry)

	foo := []byte("foo")
	bar := []byte("bar")
	far := []byte("far")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v.Value())
	v = sInterner.LoadOrStore(bar)
	assert.Equal("bar", v.Value())
	v = sInterner.LoadOrStore(far)
	assert.Equal("far", v.Value())
	v = sInterner.LoadOrStore(boo)
	assert.Equal("boo", v.Value())
}

func TestInternLoadOrStoreHandleIdentity(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(tagset.NewTable(4), 1, stringInternerTelemetry)

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

	v := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v.Value())
	v2 := sInterner.LoadOrStore(foo)
	assert.Equal(v, v2, "same value must give the same handle")
	v2 = sInterner.LoadOrStore(bar)
	assert.NotEqual(v, v2, "different values must give different handles")
	v3 := sInterner.LoadOrStore(bar)
	assert.Equal(v2, v3, "same value must give the same handle")

	v4 := sInterner.LoadOrStore(boo)
	assert.NotEqual(v, v4, "different values must give different handles")
	assert.NotEqual(v2, v4, "different values must give different handles")
	assert.NotEqual(v3, v4, "different values must give different handles")
}

// Handles issued before the lookaside cache is reset stay canonical, so the
// agent never holds two copies of the same tag. The old interner reset dropped
// its strings and started allocating fresh copies.
func TestInternHandlesSurviveReset(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(tagset.NewTable(2), 1, stringInternerTelemetry)

	before := sInterner.LoadOrStore([]byte("tag:value"))

	// force at least one reset of the lookaside cache
	for i := 0; i < 8; i++ {
		sInterner.LoadOrStore([]byte(fmt.Sprintf("filler:%d", i)))
	}

	after := sInterner.LoadOrStore([]byte("tag:value"))
	assert.Equal(before, after, "handle must stay canonical across a cache reset")
	assert.Equal(before.Hash(), after.Hash())
}

func TestInternLoadOrStoreGrowsPastSizeHint(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	// the size argument is only a pre-allocation hint now, not a cap
	sInterner := newStringInterner(tagset.NewTable(4), 1, stringInternerTelemetry)

	for i := 0; i < 64; i++ {
		sInterner.LoadOrStore([]byte(fmt.Sprintf("tag:%d", i)))
	}
	assert.Equal(64, sInterner.table.Len(), "the interner must not evict tags that are still arriving")

	// and repeats keep hitting rather than re-interning
	first := sInterner.LoadOrStore([]byte("tag:0"))
	second := sInterner.LoadOrStore([]byte("tag:0"))
	assert.Same(first, second)
	assert.Equal(64, sInterner.table.Len())
}
