// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// There are multiple instances of the interner, one per worker (depends on # of virtual CPUs).
	// Most metrics are tagged with the instance ID, however some are left as global
	// Note `New` vs `NewSimple`
	tlmSIResets = telemetry.NewCounter("dogstatsd", "string_interner_resets", []string{"interner_id"},
		"Amount of resets of the string interner used in dogstatsd")
	tlmSIRSize = telemetry.NewGauge("dogstatsd", "string_interner_entries", []string{"interner_id"},
		"Number of entries in the string interner")
	tlmSIRBytes = telemetry.NewGauge("dogstatsd", "string_interner_bytes", []string{"interner_id"},
		"Number of bytes stored in the string interner")
	tlmSIRHits = telemetry.NewCounter("dogstatsd", "string_interner_hits", []string{"interner_id"},
		"Number of times string interner returned an existing string")
	tlmSIRMiss = telemetry.NewCounter("dogstatsd", "string_interner_miss", []string{"interner_id"},
		"Number of times string interner created a new string object")
	tlmSIRNew = telemetry.NewSimpleCounter("dogstatsd", "string_interner_new",
		"Number of times string interner was created")
	tlmSIRStrBytes = telemetry.NewSimpleHistogram("dogstatsd", "string_interner_str_bytes",
		"Number of times string with specific length were added",
		[]float64{1, 2, 4, 8, 16, 32, 64, 128})
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

func newStringInterner(maxSize int, internerID int) *stringInterner {
	i := &stringInterner{
		strings: make(map[string]string),
		id:      fmt.Sprintf("interner_%d", internerID),
		maxSize: maxSize,
		telemetry: siTelemetry{
			enabled: utils.IsTelemetryEnabled(),
		},
	}

	if i.telemetry.enabled {
		i.prepareTelemetry()
	}

	return i
}

func (i *stringInterner) prepareTelemetry() {
	i.telemetry.resets = tlmSIResets.WithValues(i.id)
	i.telemetry.size = tlmSIRSize.WithValues(i.id)
	i.telemetry.bytes = tlmSIRBytes.WithValues(i.id)
	i.telemetry.hits = tlmSIRHits.WithValues(i.id)
	i.telemetry.miss = tlmSIRMiss.WithValues(i.id)
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
		tlmSIRStrBytes.Observe(float64(len(s)))
		i.telemetry.curBytes += len(s)
	}

	return s
}
