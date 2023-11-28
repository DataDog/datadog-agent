// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"fmt"
	"math"
	"os"
	"sync"
	"time"
	"unsafe"

	lru "github.com/hashicorp/golang-lru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// initialInternerSize is the size of the LRU cache (in #strings).  This is HEAP, so
// don't let this get too big compared to the MMAP region.
const initialInternerSize = 64

// backingBytesPerInCoreEntry is the number of bytes to allocate in the mmap file per
// element in our LRU.  E.g., some value of initialInternerSize * POW(GrowthFactor, N).
const backingBytesPerInCoreEntry = 4096
const defaultGrowthFactor = 1.5
const defaultMinFileSizeKb = 512

// noFileCache indicates that no mmap should be created.
const noFileCache = ""

// OriginInternal is every internal (non-container) origin.  When diagnostics
// aren't enabled, we bundle them all up into one origin.
const OriginInternal = "!Internal"

// OriginTimeSampler marks allocations to the Time Sampler.
const OriginTimeSampler = "!Timesampler"

// OriginContextResolver marks allocations to the Context Resolver.
const OriginContextResolver = "!ContextResolver"

// OriginCheckSampler marks allocations to the check sampler.
const OriginCheckSampler = "!CheckSampler"

// OriginBufferedAggregator marks allocations to the BufferedAggregator.
const OriginBufferedAggregator = "!BufferedAggregator"

// OriginContextLimiter marks allocations to the Context Limiter.
const OriginContextLimiter = "!OriginContextLimiter"

type stringInterner struct {
	cache          lruStringCache
	fileBacking    *mmapHash // if this is nil, the string interner acts as a regular LRU string cache.
	maxStringCount int
	growthFactor   float64
	refcount       int32
	refcountLock   sync.Mutex
}

// Name of the interner - based on its origin.
func (i *stringInterner) Name() string {
	if i.fileBacking != nil {
		return i.fileBacking.Name()
	}
	return "unbacked interner"
}

// bytesPerEntry returns the number
func bytesPerEntry(maxStringCount int) int64 {
	return int64(maxStringCount * backingBytesPerInCoreEntry)
}

func newStringInterner(origin string, maxStringCount int, tmpPath string, closeOnRelease, enableDiagnostics, enableTelemetry bool, growthFactor float64) *stringInterner {
	// First version: a basic mmap'd file. Nothing fancy. Later: refcount system for
	// each interner. When the mmap goes to zero, unmap it WHEN we have a newer
	// version there.
	// Growth: When our map gets too small for our items, we will grow.  Reallocate a
	// new larger mmap and start interning from there (up to some quota we later worry about).
	// Old mmap gets removed when all strings referencing it get finalized.  New strings won't be
	// created.
	var backing *mmapHash
	var err error
	if tmpPath != noFileCache {
		backing, err = newMmapHash(origin, bytesPerEntry(maxStringCount), tmpPath, closeOnRelease, enableDiagnostics)
		if err != nil {
			log.Errorf("Failed to create MMAP hash file: %v", err)
			return nil
		}
	}
	i := &stringInterner{
		cache:          newLruStringCache(maxStringCount, origin, enableTelemetry),
		fileBacking:    backing,
		growthFactor:   growthFactor,
		maxStringCount: maxStringCount,
		refcount:       1,
	}
	log.Debugf("Created new String interner %p with mmap hash %p with max size %d", i, backing, maxStringCount)
	return i
}

// loadOrStore always returns the string from the cache, adding it into the
// cache if needed.
// If we need to store a new entry and the cache is at its maximum capacity,
// it is reset.  The origin identifies the container (and thusly, the quota)
// originating the string.
func (i *stringInterner) loadOrStore(key []byte) string {
	if len(key) == 0 {
		// Empty key
		return ""
	}
	if i.maxStringCount == 0 {
		// Dead interner.  release() has already been called, or it was broken on
		// construction.
		return ""
	}

	result := i.cache.lookupOrInsert(key, func(key []byte) string {
		if i.fileBacking != nil {
			s, possible := i.fileBacking.lookupOrInsert(key)
			if len(s) == 0 {
				if !possible {
					// String is too large to allocate in the backing, just do it
					// on the heap.  Let GC take care of it normally.  The string cache
					// already de-duplicates for us.
					return string(key)
				}
				// Return the empty string here, and "loadOrStore's" caller will
				// resize the interner in response.
				return ""
			}
			return s
		}
		return string(key)
	})

	return result
}

func (i *stringInterner) used() int64 {
	used, _ := i.fileBacking.sizes()
	return used
}

// Retain some references
func (i *stringInterner) Retain(n int32) {
	i.refcountLock.Lock()
	defer i.refcountLock.Unlock()
	i.refcount += n
}

// Release some references
func (i *stringInterner) Release(n int32) {
	i.refcountLock.Lock()
	defer i.refcountLock.Unlock()
	if i.refcount < 1 {
		log.Warnf("Dead stringInterner being released!  refcount=%d", i.refcount)
		return
	}
	if i.refcount > 0 && i.refcount-n < 1 {
		log.Debugf("Finalizing backing, refcount=%d, n=%d", i.refcount, n)
		i.fileBacking.finalize()
		// a dead interner in case anyone comes looking for us again.
		i.cache = newLruStringCache(0, i.cache.origin, false)
		i.maxStringCount = 0
	}
	i.refcount -= n
}

func (i *stringInterner) retain() {
	i.refcountLock.Lock()
	defer i.refcountLock.Unlock()
	if i.refcount < 1 {
		log.Error("Dead interner being re-retained!")
	}
	i.refcount++
}

// Interner interns strings to reduce memory usage.
type Interner interface {
	LoadOrStore([]byte, string, InternRetainer) string
}

// KeyedInternerConfig holds all the read-only configuration for
type KeyedInternerConfig struct {
	MaxQuota          int
	CloseOnRelease    bool
	TmpPath           string
	MinFileSize       int64
	MaxPerInterner    int
	EnableDiagnostics bool
	GrowthFactor      float64
	EnableTelemetry   bool
}

// KeyedInterner has an origin-keyed set of interners.
type KeyedInterner struct {
	interners  *lru.Cache
	lastReport time.Time
	config     KeyedInternerConfig
	lock       sync.Mutex
}

// DefaultKeyedInternerConfig returns a reasonably-defaulted KeyedInternerConfig that can
// be used for test cases or untuned production use.
func DefaultKeyedInternerConfig() KeyedInternerConfig {
	return KeyedInternerConfig{
		MaxQuota:          -1,
		CloseOnRelease:    true,
		TmpPath:           "/",
		MinFileSize:       defaultMinFileSizeKb * 1024,
		MaxPerInterner:    initialInternerSize,
		EnableDiagnostics: false,
		GrowthFactor:      defaultGrowthFactor,
		EnableTelemetry:   false,
	}
}

// NewKeyedStringInterner takes args explicitly for initialization
func NewKeyedStringInterner(stringInternerCacheSize int, config KeyedInternerConfig) Interner {
	cache, err := lru.NewWithEvict(stringInternerCacheSize, func(_, internerUntyped interface{}) {
		interner := internerUntyped.(*stringInterner)
		interner.Release(1)
	})
	if err != nil {
		return nil
	}
	return &KeyedInterner{
		config:     config,
		interners:  cache,
		lastReport: time.Now(),
	}
}

// NewKeyedStringInternerMemOnly is a memory-only cache with no disk needs.
func NewKeyedStringInternerMemOnly(stringInternerCacheSize int) Interner {
	cache, err := lru.NewWithEvict(stringInternerCacheSize, func(_, internerUntyped interface{}) {
		interner := internerUntyped.(*stringInterner)
		interner.Release(1)
	})
	config := DefaultKeyedInternerConfig()
	config.MaxPerInterner = stringInternerCacheSize
	if err != nil {
		return nil
	}
	return &KeyedInterner{
		interners:  cache,
		lastReport: time.Now(),
		config:     config,
	}
}

// SetTelemetry sets the 'telemetry enabled' flag.
func (i *KeyedInterner) SetTelemetry(enableTelemetry bool) {
	i.config.EnableTelemetry = enableTelemetry
}

// 'static' globals for query statistics.
var sGlobalQueryCount = atomic.NewInt64(0)
var sFailedInternalCount = atomic.NewInt64(0)

// LoadOrStoreString interns a string for an origin
func (i *KeyedInterner) LoadOrStoreString(s string, origin string, retainer InternRetainer) string {
	if Check(s) {
		return i.LoadOrStore(unsafe.Slice(unsafe.StringData(s), len(s)), origin, retainer)
	}
	sFailedInternalCount.Add(1)
	return "<invalid>"
}

// LoadOrStore interns a byte-array to a string, for an origin
func (i *KeyedInterner) LoadOrStore(key []byte, origin string, retainer InternRetainer) string {
	sGlobalQueryCount.Add(1)
	keyLen := len(key)
	// Avoid locking for dumb stuff.
	if keyLen == 0 {
		return ""
	} else if keyLen > MaxValueSize {
		// These objects are too big to fit
		return string(key)
	}
	return i.loadOrStore(key, origin, retainer)
}

func (i *KeyedInterner) makeInterner(origin string, stringMaxCount int) *stringInterner {
	if bytesPerEntry(stringMaxCount) >= i.config.MinFileSize {
		return newStringInterner(origin, stringMaxCount, i.config.TmpPath, i.config.CloseOnRelease, i.config.EnableDiagnostics, i.config.EnableTelemetry, i.config.GrowthFactor)
	}
	// No file cache until we get bigger.
	return newStringInterner(origin, stringMaxCount, noFileCache, i.config.CloseOnRelease, i.config.EnableDiagnostics, i.config.EnableTelemetry, i.config.GrowthFactor)
}

func (i *KeyedInterner) loadOrStore(key []byte, origin string, retainer InternRetainer) string {
	// The mutex usage is pretty rudimentary.  Upon profiling, have a look at better synchronization.
	// E.g., lock-free LRU.
	i.lock.Lock()
	defer i.lock.Unlock()

	// When diagnostics are off, bucket all non-container origins into one.
	if !i.config.EnableDiagnostics && len(origin) > 0 && origin[0] == '!' {
		origin = OriginInternal
	}

	if i.config.EnableDiagnostics && i.lastReport.Before(time.Now().Add(-1*time.Minute)) {
		log.Debugf("*** INTERNER *** Keyed Interner has %d interners.  CloseOnRelease=%v, Total Query Count: %v, Total Failures: %v", i.interners.Len(), i.config.CloseOnRelease,
			sGlobalQueryCount.Load(), sFailedInternalCount.Load())
		Report()
		i.lastReport = time.Now()
	}

	var interner *stringInterner
	if i.interners.Contains(origin) {
		internerUntyped, _ := i.interners.Get(origin)
		interner = internerUntyped.(*stringInterner)

		// TODO: this is where you enforce container quota limits.
		if i.config.MaxQuota > 0 && interner.used() >= int64(i.config.MaxQuota) {
			_, _ = fmt.Fprintf(os.Stderr, "Over quota on (%d) origin %s\n", i.config.MaxQuota, origin)
			return ""
		}
	} else {
		interner = i.makeInterner(origin, i.config.MaxPerInterner)
		if i.config.EnableDiagnostics {
			log.Debugf("Creating string interner at %p for origin %v", interner, origin)
		}
		i.interners.Add(origin, interner)
	}

	ret := interner.loadOrStore(key)
	if ret == "" {
		// The only way the interner won't return a string is if it's full.  Make a new bigger one and
		// start using that. We'll eventually migrate all the in-use strings to this from this container.
		if i.config.EnableDiagnostics {
			log.Debugf("Failed interning string.  Adding new interner for key %v, length %v", string(key), len(key))
		}
		replacementInterner := i.makeInterner(origin, int(math.Ceil(float64(interner.maxStringCount)*i.config.GrowthFactor)))
		if replacementInterner == nil {
			// We couldn't intern the string nor create a new interner, so just heap allocate.  newStringInterner
			// will log errors when it fails like this.
			return string(key)
		}
		// We have one retention on the interner upon creation, to keep it available for ongoing use.
		// Release that now.
		i.interners.Add(origin, replacementInterner)
		retainer.Reference(replacementInterner)
		replacementInterner.retain()
		if i.config.EnableDiagnostics {
			log.Debugf("Releasing old interner.  Prior: %p -> New: %p", interner, replacementInterner)
		}
		interner.Release(1)
		return replacementInterner.loadOrStore(key)
	}
	interner.retain()
	if retainer != nil {
		retainer.Reference(interner)
	}
	return ret
}

// InternerContext saves all the arguments to LoadOrStore to avoid passing separately through function
// calls.
type InternerContext struct {
	interner *KeyedInterner
	origin   string
	retainer InternRetainer
}

// NewInternerContext creates a new one, binding the args for future calls to LoadOrStore
func NewInternerContext(interner *KeyedInterner, origin string, retainer InternRetainer) InternerContext {
	return InternerContext{
		interner: interner,
		origin:   origin,
		retainer: retainer,
	}
}

// UseStringBytes calls LoadOrStore on the saved interner with the saved arguments.  Add
// the given suffix to the origin.
func (i *InternerContext) UseStringBytes(s []byte, suffix string) string {
	// TODO: Assume that the string is almost certainly already intern'd.
	if i == nil {
		return string(s)
	}
	return i.interner.LoadOrStore(s, i.origin+suffix, i.retainer)
}
