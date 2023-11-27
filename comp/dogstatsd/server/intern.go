// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

var (
	// There are multiple instances of the interner, one per worker (depends on # of virtual CPUs).
	// Most metrics are tagged with the instance ID, however some are left as global
	// Note `New` vs `NewSimple`
	tlmSIResets = telemetry.NewCounter("dogstatsd", "string_interner_resets", []string{"interner_id"},
		"Amount of resets of the string interner used in dogstatsd")
)

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
type stringInterner struct {
	strings map[string]string
	maxSize int
	id      string

	telemetry siTelemetry
}

type siTelemetry struct {
	enabled  bool
	curBytes int

	resets telemetry.SimpleCounter
	size   telemetry.SimpleGauge
	bytes  telemetry.SimpleGauge
	hits   telemetry.SimpleCounter
	miss   telemetry.SimpleCounter
}

func newStringInterner(maxSize int, internerID int, enableTelemetry bool) *stringInterner {
	i := &stringInterner{
		strings: make(map[string]string),
		id:      fmt.Sprintf("interner_%d", internerID),
		maxSize: maxSize,
		telemetry: siTelemetry{
			enabled: enableTelemetry,
		},
	}

	if i.telemetry.enabled {
		i.prepareTelemetry()
	}

	return i
}

func (i *stringInterner) prepareTelemetry() {
	i.telemetry.resets = tlmSIResets.WithValues(i.id)
	i.telemetry.size = cache.TlmSIRSize.WithValues(i.id)
	i.telemetry.bytes = cache.TlmSIRBytes.WithValues(i.id)
	i.telemetry.hits = cache.TlmSIRHits.WithValues(i.id)
	i.telemetry.miss = cache.TlmSIRMiss.WithValues(i.id)
}

func (i *stringInterner) LoadOrStore(b []byte, origin string, retainer cache.InternRetainer) string {
	return i.loadOrStore(b)
}

// LoadOrStore always returns the string from the cache, adding it into the
// cache if needed.
// If we need to store a new entry and the cache is at its maximum capacity,
// it is reset.
func (i *stringInterner) loadOrStore(key []byte) string {
	// here is the string interner trick: the map lookup using
	// string(key) doesn't actually allocate a string, but is
	// returning the string value -> no new heap allocation
	// for this string.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if s, found := i.strings[string(key)]; found {
		if i.telemetry.enabled {
			i.telemetry.hits.Inc()
		}
		return s
	}
	if len(i.strings) >= i.maxSize {
		if i.telemetry.enabled {
			i.telemetry.resets.Inc()
			i.telemetry.bytes.Sub(float64(i.telemetry.curBytes))
			i.telemetry.size.Sub(float64(len(i.strings)))
			i.telemetry.curBytes = 0
		}

		i.strings = make(map[string]string)
	}

	s := string(key)
	i.strings[s] = s

	if i.telemetry.enabled {
		i.telemetry.miss.Inc()
		i.telemetry.size.Inc()
		i.telemetry.bytes.Add(float64(len(s)))
		cache.TlmSIRStrBytes.Observe(float64(len(s)))
		i.telemetry.curBytes += len(s)
	}

	return s
}
