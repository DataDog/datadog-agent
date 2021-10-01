package context_resolver

import (
	"bytes"
	"encoding/gob"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/dgraph-io/badger/v3"
	"log"
	"os"
	"time"
)

// ContextResolver allows tracking and expiring contexts
type contextResolverBadger struct {
	contextResolverBase

	db            *badger.DB
	ticker        *time.Ticker
	contextsByKey map[ckey.ContextKey]*Context
}

func (cr *contextResolverBadger) serializeContextKey(key ckey.ContextKey) []byte {
	return key.ToBytes()
}

// TODO: we probably want to encode it manually to be a bit more efficient here.
func (cr *contextResolverBadger) serializeContext(c *Context) []byte {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(*c)
	if err != nil {
		log.Fatal(err)
	}
	return buffer.Bytes()
}

func (cr *contextResolverBadger) deserializeContext(b []byte) *Context {
	buffer := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buffer)
	c := &Context{}
	err := dec.Decode(&c)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

// NewContextResolverBadger creates a new context resolver using badger to store contexts
func NewContextResolverBadger(inMemory bool, path string) *contextResolverBadger {
	opt := badger.DefaultOptions(path).WithInMemory(inMemory).WithCompactL0OnClose(false)
	opt = opt.WithLevelSizeMultiplier(10).WithMaxLevels(5)
	opt = opt.WithNumMemtables(1).WithMemTableSize(10 << 20).WithBaseLevelSize(5 << 20)
	opt = opt.WithValueLogMaxEntries(100000)
	opt = opt.WithBlockCacheSize(32 << 20)
	opt = opt.WithValueLogFileSize(2 << 20)

	if !inMemory {
		// We never want to re-use existing files.
		e := os.Remove(path)
		if e != nil {
			log.Print(e)
		}
		opt.WithSyncWrites(false).WithDetectConflicts(false)
	}
	db, err := badger.Open(opt)
	if err != nil {
		log.Fatal(err)
	}
	ticker := time.NewTicker(1 * time.Minute)

	cr := &contextResolverBadger{
		contextResolverBase: contextResolverBase {
			keyGenerator:  ckey.NewKeyGenerator(),
			tagsBuffer:    util.NewTagsBuilder(),
		},
		db:            db,
		ticker:        ticker,
		contextsByKey: make(map[ckey.ContextKey]*Context),
	}
	go cr.runGC()
	return cr
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolverBadger) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.tagsBuffer)               // tags here are not sorted and can contain duplicates
	contextKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates from cr.tagsBuffer (and doesn't mind the order)

	if _, ok := cr.contextsByKey[contextKey]; !ok {
		// making a copy of tags for the context since tagsBuffer
		// will be reused later. This allows us to allocate one slice
		// per context instead of one per sample.
		c := &Context{
			Name: metricSampleContext.GetName(),
			Tags: cr.tagsBuffer.Copy(),
			Host: metricSampleContext.GetHost(),
		}
		err := cr.db.Update(func(txn *badger.Txn) error {
			err := txn.Set(cr.serializeContextKey(contextKey), cr.serializeContext(c))
			return err
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	cr.tagsBuffer.Reset()
	return contextKey
}

// Get gets a context resolver for a given key
func (cr *contextResolverBadger) Get(key ckey.ContextKey) (*Context, bool) {
	var context *Context

	// FIXME: review error handling.
	err := cr.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(cr.serializeContextKey(key))
		if err != nil {
			return err
		}
		if item.IsDeletedOrExpired() {
			return nil
		}
		err = item.Value(func(val []byte) error {
			context = cr.deserializeContext(val)
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, false
	}

	return context, context != nil
}

// Size returns the number of contexts stored
func (cr *contextResolverBadger) Size() int {
	count := 0
	err := cr.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if it.Item().IsDeletedOrExpired() {
				continue
			}
			count++

		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return count
}

// removeKeys remove all the contexts matching the keys
func (cr *contextResolverBadger) removeKeys(expiredContextKeys []ckey.ContextKey) {
	err := cr.db.Update(func(txn *badger.Txn) error {
		for _, expiredContextKey := range expiredContextKeys {
			err := txn.Delete(cr.serializeContextKey(expiredContextKey))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func (cr *contextResolverBadger) runGC() {
	for range cr.ticker.C {
	again:
		err := cr.db.RunValueLogGC(0.7)
		if err == nil {
			goto again
		}
	}
}

// Clear drops all contexts
func (cr *contextResolverBadger) Clear() {
	err := cr.db.DropAll()
	if err != nil {
		log.Fatal(err)
	}
}

// Close frees up resources
func (cr *contextResolverBadger) Close() {
	cr.ticker.Stop()
	err := cr.db.Close()
	if err != nil {
		log.Fatal(err)
	}
}
