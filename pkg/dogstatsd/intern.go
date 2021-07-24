package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// Amount of entries in the string interner used in dogstatsd.
	// Note that it's not ideal because there are multiple string interners
	// (one per worker) but this will still give us an insight (and it's
	// comparable as long as the amount of worker is stable).
	tlmSIEntries = telemetry.NewGauge("dogstatsd", "string_interner_entries",
		nil, "Amount of entries in the string interner used in dogstatsd")
)

const (
	// dropInterval controls how frequently an entry is dropped, regardless of map size.
	// Specifically, an entry is dropped every dropInterval calls to LoadOrStore.
	// This ensures that eventually old entries are evicted.
	// It is recommended, for performance, to use a number that is power-of-2.
	dropInterval = 256
)

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
type stringInterner struct {
	strings map[string]string
	maxSize int
	calls uint64
	// telemetry
	tlmEnabled bool
}

func newStringInterner(maxSize int) *stringInterner {
	return &stringInterner{
		strings:    make(map[string]string),
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
	// TODO: Use a LRU/LFU policy (possibly approximated via sampling).
	i.calls++
	if i.calls % dropInterval == 0 {
		for k := range i.strings {
			if k == s {
				continue
			}
			delete(i.strings, k)
			break
		}
		tlmSIEntries.Set(len(i.strings))
	}
	
	// Silly case: it's pointless to use/lookup an entry for this.
	if len(key) == 0 {
		return ""
	}
		
	// This is the string interner trick: the map lookup using
	// string(key) doesn't actually allocate a string, but is
	// returning the string value -> no new heap allocation
	// for this string.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if s, found := i.strings[string(key)]; found {
		return s
	}
	
	// If adding a new entry would bring us over the maxSize limit, randomly drop one entry.
	// TODO: Use a LRU/LFU policy (possibly approximated via sampling).
	if len(i.strings) >= i.maxSize {
		for k := range i.strings {
			delete(i.strings, k)
			break
		}
	}
	
	// Add the new entry.
	s := string(key)
	i.strings[s] = s
	
	tlmSIEntries.Set(len(i.strings))

	return s
}
