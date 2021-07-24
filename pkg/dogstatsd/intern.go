package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
)

var (
	// Amount of entries in the string interner used in dogstatsd.
	// Note that it's not ideal because there are multiple string interners
	// (one per worker) but this will still give us an insight (and it's
	// comparable as long as the amount of worker is stable).
	tlmSIEntries = telemetry.NewGauge("dogstatsd", "string_interner_entries",
		nil, "Amount of entries in the dogstasts string interner")
	// Number of misses to strings in the interner.
	// Together with tlmSIHits can be used to calculate the hit ratio.
	tlmSIMiss = telemetry.NewCounter("dogstatsd", "string_interner_miss",
		nil, "Number of misses in the dogstatsd string interner")
	// Number of hits to strings already in the interner.
	// Together with tlmSIMiss can be used to calculate the hit ratio.
	tlmSIHits = telemetry.NewCounter("dogstatsd", "string_interner_hits",
		nil, "Number of hits in the dogstatsd string interner")
)

const (
	// dropInterval controls how frequently an entry is dropped, regardless of map size.
	// Specifically, an entry is dropped every dropInterval calls to LoadOrStore.
	// This ensures that eventually old entries are evicted.
	// It is recommended, for performance, to use a number that is power-of-2.
	dropInterval = 256
	// dropSample controls how many entries are sampled, at most, to pick the least
	// recently used when an entry needs to be dropped.
	// Higher numbers lower the chances of recently-accessed entries to be dropped,
	// at the expense of higher CPU usage.
	dropSample = 3
)

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
type stringInterner struct {
	strings map[string]*stringEntry
	maxSize int
	calls   uint
	// telemetry
	tlmEnabled bool
}

type stringEntry struct {
	str        string
	lastAccess uint
}

func newStringInterner(maxSize int) *stringInterner {
	return &stringInterner{
		strings:    make(map[string]*stringEntry),
		maxSize:    maxSize,
		tlmEnabled: telemetry_utils.IsEnabled(),
	}
}

// LoadOrStore always returns the string from the cache, adding it into the
// cache if needed.
// If we need to store a new entry and the cache is at its maximum capacity,
// an existing entry is randomly dropped.
func (i *stringInterner) LoadOrStore(key []byte) string {
	// Drop one random entry every dropInterval calls to LoadOrStore. This
	// aims at ensuring that entries are eventually dropped even if no new
	// entries are added.
	i.calls++
	if i.calls%dropInterval == 0 {
		// Drop a old entry, avoding the one we are going to need below.
		i.dropOldEntry(key)
	}

	// Silly case: it's pointless to use/lookup an entry for this.
	if len(key) == 0 {
		if i.tlmEnabled {
			tlmSIHits.Inc()
		}
		return ""
	}

	// This is the string interner trick: the map lookup using
	// string(key) does not actually allocate a string, but is
	// returning the string value -> no new heap allocation
	// for this string.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if entry, found := i.strings[string(key)]; found {
		entry.lastAccess = i.calls
		if i.tlmEnabled {
			tlmSIHits.Inc()
		}
		return entry.str
	}

	// If adding a new entry would bring us over the maxSize limit, randomly drop one entry.
	if len(i.strings) >= i.maxSize {
		i.dropOldEntry(nil)
	}

	// Add the new entry.
	s := string(key)
	i.strings[s] = &stringEntry{str: s, lastAccess: i.calls}
	if i.tlmEnabled {
		tlmSIEntries.Inc()
		tlmSIMiss.Inc()
	}

	return s
}

func (i *stringInterner) dropOldEntry(excludedKey []byte) {
	var victim *stringEntry
	sample := dropSample
	for _, entry := range i.strings {
		if string(excludedKey) == entry.str { // Does not allocate.
			// We do not consider this entry for deletion.
			continue
		}
		if sample <= 0 {
			break
		}
		sample--
		if victim == nil || victim.lastAccess > entry.lastAccess {
			// Either we have no victim yet, or the current victim has been
			// accessed more recently than the current entry.
			victim = entry
		}
	}
	if victim != nil {
		delete(i.strings, victim.str)
		if i.tlmEnabled {
			tlmSIEntries.Dec()
		}
	}
}
