package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// Amount of resets of the string interner used in dogstatsd
	// Note that it's not ideal because there is many allocated string interner
	// (one per worker) but it'll still give us an insight (and it's comparable
	// as long as the amount of worker is stable).
	tlmSIResets = telemetry.NewCounter("dogstatsd", "string_interner_resets",
		nil, "Amount of resets of the string interner used in dogstatsd")
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
			delete(i.strings, k)
			break
		}
	}
	
	if len(key) == 0 {
		return ""
	}
		
	// here is the string interner trick: the map lookup using
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
	return s
}
