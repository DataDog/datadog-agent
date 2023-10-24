// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"fmt"
	cconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	lru "github.com/hashicorp/golang-lru"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// initialInternerSize is the size of the LRU cache (in #strings).  This is HEAP, so
// don't let this get too big compared to the MMAP region.
const initialInternerSize = 64

// TODO: Setup min size before using mmap, setup max mmap size (to effectively force
// copying GC of strings).
// backingBytesPerInCoreEntry is the number of bytes to allocate in the mmap file per
// element in our LRU.  E.g., some value of initialInternerSize * POW(growthFactor, N).
const backingBytesPerInCoreEntry = 4096
const growthFactor = 1.5

// TODO: Move this to a config arg.
const defaultTmpPath = "/tmp"
const OriginTimeSampler = "!Timesampler"
const OriginContextResolver = "!ContextResolver"
const OriginCheckSampler = "!CheckSampler"
const OriginBufferedAggregator = "!BufferedAggregator"
const OriginContextLimiter = "!OriginContextLimiter"

type stringInterner struct {
	cache        lruStringCache
	fileBacking  *mmap_hash // if this is nil, the string interner acts as a regular LRU string cache.
	maxSize      int
	refcount     int32
	refcountLock sync.Mutex
}

func newStringInterner(origin string, maxSize int, tmpPath string, closeOnRelease bool) *stringInterner {
	// First version: a basic mmap'd file. Nothing fancy. Later: refcount system for
	// each interner. When the mmap goes to zero, unmap it WHEN we have a newer
	// version there.
	// Growth: When our map gets too small for our items, we will grow.  Reallocate a
	// new larger mmap and start interning from there (up to some quota we later worry about).
	// Old mmap gets removed when all strings referencing it get finalized.  New strings won't be
	// created.
	var backing *mmap_hash = nil
	var err error = nil
	if tmpPath != "" {
		backing, err = newMmapHash(origin, int64(maxSize*backingBytesPerInCoreEntry), tmpPath, closeOnRelease)
		if err != nil {
			return nil
		}
	}
	i := &stringInterner{
		cache:       newLruStringCache(maxSize, false),
		fileBacking: backing,
		maxSize:     maxSize,
		refcount:    1,
	}
	log.Warnf("Created new String interner %p with mmap hash %p with max size %d", i, backing, maxSize)
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
	if i.maxSize == 0 {
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
				} else {
					return ""
				}
			} else {
				return s
			}
		} else {
			return string(key)
		}
	})

	return result
}

func (i *stringInterner) used() int64 {
	used, _ := i.fileBacking.sizes()
	return used
}

func (i *stringInterner) Release(n int32) {
	i.refcountLock.Lock()
	defer i.refcountLock.Unlock()
	if i.refcount < 1 {
		log.Infof("Dead stringInterner begin released!  refcount=%d", i.refcount)
		return
	}
	if i.refcount > 0 && i.refcount-n < 1 {
		log.Infof("Finalizing backing, refcount=%d, n=%d", i.refcount, n)
		i.fileBacking.finalize()
		i.cache = newLruStringCache(0, false)
		i.maxSize = 0
	}
	i.refcount -= n
}

func (i *stringInterner) retain() {
	i.refcountLock.Lock()
	defer i.refcountLock.Unlock()
	if i.refcount < 1 {
		log.Errorf("Dead interner being re-retained!")
	}
	i.refcount += 1
}

type KeyedInterner struct {
	interners      *lru.Cache
	maxQuota       int
	closeOnRelease bool
	tmpPath        string
	lastReport     time.Time
	lock           sync.Mutex
}

// NewKeyedStringInterner creates a Keyed String Interner with a max per-origin quota of maxQuota
func NewKeyedStringInterner(cfg cconfig.Component) *KeyedInterner {
	closeOnRelease := !cfg.GetBool("dogstatsd_string_interner_preserve_mmap")
	stringInternerCacheSize := cfg.GetInt("dogstatsd_string_interner_size")

	return NewKeyedStringInternerVals(stringInternerCacheSize, closeOnRelease)
}

func NewKeyedStringInternerVals(stringInternerCacheSize int, closeOnRelease bool) *KeyedInterner {
	cache, err := lru.NewWithEvict(stringInternerCacheSize, func(_, internerUntyped interface{}) {
		interner := internerUntyped.(*stringInterner)
		interner.Release(1)
	})
	if err != nil {
		return nil
	}
	return &KeyedInterner{
		interners:      cache,
		maxQuota:       -1,
		lastReport:     time.Now(),
		tmpPath:        defaultTmpPath,
		closeOnRelease: closeOnRelease,
	}
}

// NewKeyedStringInternerMemOnly is a memory-only cache with no disk needs.
func NewKeyedStringInternerMemOnly(stringInternerCacheSize int) *KeyedInterner {
	cache, err := lru.NewWithEvict(stringInternerCacheSize, func(_, internerUntyped interface{}) {
		interner := internerUntyped.(*stringInterner)
		interner.Release(1)
	})
	if err != nil {
		return nil
	}
	return &KeyedInterner{
		interners:      cache,
		maxQuota:       -1,
		lastReport:     time.Now(),
		tmpPath:        "",
		closeOnRelease: false,
	}
}

var s_globalQueryCount uint64 = 0

func (i *KeyedInterner) LoadOrStoreString(s string, origin string, retainer InternRetainer) string {
	return i.LoadOrStore(unsafe.Slice(unsafe.StringData(s), len(s)), origin, retainer)
}

func (i *KeyedInterner) LoadOrStore(key []byte, origin string, retainer InternRetainer) string {
	atomic.AddUint64(&s_globalQueryCount, 1)
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

func (i *KeyedInterner) loadOrStore(key []byte, origin string, retainer InternRetainer) string {
	// The mutex usage is pretty rudimentary.  Upon profiling, have a look at better synchronization.
	// E.g., lock-free LRU.
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.lastReport.Before(time.Now().Add(-1 * time.Minute)) {
		log.Infof("*** INTERNER *** Keyed Interner has %d interners.  closeOnRelease=%v, Total Query Count: %v", i.interners.Len(), i.closeOnRelease,
			atomic.LoadUint64(&s_globalQueryCount))
		Report()
		i.lastReport = time.Now()
	}
	var interner *stringInterner = nil
	if i.interners.Contains(origin) {
		internerUntyped, _ := i.interners.Get(origin)
		interner = internerUntyped.(*stringInterner)

		// TODO: this is where you enforce container quota limits.
		if i.maxQuota > 0 && interner.used() >= int64(i.maxQuota) {
			_, _ = fmt.Fprintf(os.Stderr, "Over quota on (%d) origin %s\n", i.maxQuota, origin)
			return ""
		}
	} else {
		interner = newStringInterner(origin, initialInternerSize, i.tmpPath, i.closeOnRelease)
		_ = log.Warnf("Creating string interner at %p for origin %v", interner, origin)
		i.interners.Add(origin, interner)
	}

	if ret := interner.loadOrStore(key); ret == "" {
		// The only way the interner won't return a string is if it's full.  Make a new bigger one and
		// start using that. We'll eventually migrate all the in-use strings to this from this container.
		log.Infof("Failed interning string.  Adding new interner for key %v, length %v", string(key), len(key))
		Report()
		replacementInterner := newStringInterner(origin, int(math.Ceil(float64(interner.maxSize)*growthFactor)), i.tmpPath, i.closeOnRelease)

		// We have one retention on the interner upon creation, to keep it available for ongoing use.
		// Release that now.
		i.interners.Add(origin, replacementInterner)
		retainer.Reference(replacementInterner)
		replacementInterner.retain()
		log.Infof("Releasing old interner.  Prior: %p -> New: %p", interner, replacementInterner)
		interner.Release(1)
		return replacementInterner.loadOrStore(key)
	} else {
		interner.retain()
		retainer.Reference(interner)
		return ret
	}
}

func (i *KeyedInterner) Release(retainer InternRetainer) {
	retainer.ReleaseAllWith(func(obj Refcounted, count int32) {
		obj.Release(count)
	})
}

type InternerContext struct {
	interner *KeyedInterner
	origin   string
	retainer InternRetainer
}

func NewInternerContext(interner *KeyedInterner, origin string, retainer InternRetainer) InternerContext {
	return InternerContext{
		interner: interner,
		origin:   origin,
		retainer: retainer,
	}
}

func (i *InternerContext) UseStringBytes(s []byte) string {
	// TODO: Assume that the string is almost certainly already intern'd.
	// TODO: Validate here.
	//s = CheckDefault(s)
	return i.interner.LoadOrStore(s, i.origin, i.retainer)
}

func (i *InternerContext) UseString(s string) string {
	s = CheckDefault(s)
	// TODO: Assume that the string is almost certainly already intern'd.
	return i.interner.LoadOrStore(unsafe.Slice(unsafe.StringData(s), len(s)), i.origin, i.retainer)
}
