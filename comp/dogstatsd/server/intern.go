// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
)

var (
	// There are multiple instances of the interner, one per worker. Counters are normally fine,
	// gauges require special care to make sense. We don't need to clean up when an instance is
	// dropped, because it only happens on agent shutdown.
	tlmSIResets = telemetry.NewSimpleCounter("dogstatsd", "string_interner_resets",
		"Amount of resets of the string interner used in dogstatsd")
	tlmSIRSize = telemetry.NewSimpleGauge("dogstatsd", "string_interner_entries",
		"Number of entries in the string interner")
	tlmSIRBytes = telemetry.NewSimpleGauge("dogstatsd", "string_interner_bytes",
		"Number of bytes stored in the string interner")
	tlmSIRHits = telemetry.NewSimpleCounter("dogstatsd", "string_interner_hits",
		"Number of times string interner returned an existing string")
	tlmSIRMiss = telemetry.NewSimpleCounter("dogstatsd", "string_interner_miss",
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
	strings    map[string]string
	maxSize    int
	curBytes   int
	tlmEnabled bool
}

func newStringInterner(maxSize int) *stringInterner {
	i := &stringInterner{
		strings:    make(map[string]string),
		maxSize:    maxSize,
		tlmEnabled: telemetry_utils.IsEnabled(),
	}
	if i.tlmEnabled {
		tlmSIRNew.Inc()
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
		if i.tlmEnabled {
			tlmSIRHits.Inc()
		}
		return s
	}
	if len(i.strings) >= i.maxSize {
		if i.tlmEnabled {
			tlmSIResets.Inc()
			tlmSIRBytes.Sub(float64(i.curBytes))
			tlmSIRSize.Sub(float64(len(i.strings)))
			i.curBytes = 0
		}

		i.strings = make(map[string]string)
		log.Debug("clearing the string interner cache")

	}

	s := string(key)
	i.strings[s] = s

	if i.tlmEnabled {
		tlmSIRMiss.Inc()
		tlmSIRSize.Inc()
		tlmSIRBytes.Add(float64(len(s)))
		tlmSIRStrBytes.Observe(float64(len(s)))
		i.curBytes += len(s)
	}

	return s
}
