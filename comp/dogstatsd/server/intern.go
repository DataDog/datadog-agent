// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
)

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
//
// The current interning strategy is fairly simple, but can require manual
// adjustments of the `maxSize` to improve performance, which is not ideal.

// However the current strategy works well enough, and there is an
// accepted go proposal to offer an "interning" mechanism from the
// go runtime directly.

// Once this is available, the interner design should be re-visited to
// take advantage of the new "Unique" api that is proposed below.
// ref: https://github.com/golang/go/issues/62483
type stringInterner struct {
	strings map[string]string
	maxSize int
	id      string

	telemetry *stringInternerInstanceTelemetry
}

func newStringInterner(maxSize int, internerID int, siTelemetry *stringInternerTelemetry) *stringInterner {
	// telemetryOnce.Do(func() { initGlobalTelemetry(telemetrycomp) })

	id := fmt.Sprintf("interner_%d", internerID)
	i := &stringInterner{
		strings:   make(map[string]string),
		id:        id,
		maxSize:   maxSize,
		telemetry: siTelemetry.PrepareForID(id),
	}

	return i
}

// LoadOrStore always returns the string from the cache, adding it into the
// cache if needed.
// If we need to store a new entry and the cache is at its maximum capacity,
// it is reset.
func (i *stringInterner) LoadOrStore(key []byte) string {
	// here is the string interner trick: the map lookup using
	// string(key) doesn't actually allocate a string, but is
	// returning the string value -> no new heap allocation
	// for this string.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if s, found := i.strings[string(key)]; found {
		i.telemetry.Hit()
		return s
	}

	if len(i.strings) >= i.maxSize {
		i.telemetry.Reset(len(i.strings))

		i.strings = make(map[string]string)
	}

	s := string(key)
	i.strings[s] = s

	i.telemetry.Miss(len(s))

	return s
}
