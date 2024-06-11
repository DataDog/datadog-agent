// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

var (
	tlmSIRStrBytes telemetry.SimpleHistogram
	telemtryOnce   sync.Once
)

func initGlobalTelemetry(telemtrycomp telemetry.Component) {
	tlmSIRStrBytes = telemtrycomp.NewSimpleHistogram("dogstatsd", "string_interner_str_bytes",
		"Number of times string with specific length were added",
		[]float64{1, 2, 4, 8, 16, 32, 64, 128})
}

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

func newStringInterner(maxSize int, internerID int, telemetrycomp telemetry.Component) *stringInterner {
	telemtryOnce.Do(func() { initGlobalTelemetry(telemetrycomp) })

	i := &stringInterner{
		strings: make(map[string]string),
		id:      fmt.Sprintf("interner_%d", internerID),
		maxSize: maxSize,
		telemetry: siTelemetry{
			enabled: utils.IsTelemetryEnabled(config.Datadog()),
		},
	}

	if i.telemetry.enabled {
		i.prepareTelemetry(telemetrycomp)
	}

	return i
}

func (i *stringInterner) prepareTelemetry(telemetrycomp telemetry.Component) {
	i.telemetry.resets = telemetrycomp.NewCounter("dogstatsd", "string_interner_resets", []string{"interner_id"},
		"Amount of resets of the string interner used in dogstatsd").WithValues(i.id)
	i.telemetry.size = telemetrycomp.NewGauge("dogstatsd", "string_interner_entries", []string{"interner_id"},
		"Number of entries in the string interner").WithValues(i.id)
	i.telemetry.bytes = telemetrycomp.NewGauge("dogstatsd", "string_interner_bytes", []string{"interner_id"},
		"Number of bytes stored in the string interner").WithValues(i.id)
	i.telemetry.hits = telemetrycomp.NewCounter("dogstatsd", "string_interner_hits", []string{"interner_id"},
		"Number of times string interner returned an existing string").WithValues(i.id)
	i.telemetry.miss = telemetrycomp.NewCounter("dogstatsd", "string_interner_miss", []string{"interner_id"},
		"Number of times string interner created a new string object").WithValues(i.id)
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
