package aggregator

import (
	"bytes"
	"encoding/gob"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/dgraph-io/badger/v3"
	"log"
	"time"
)

// contextResolver allows tracking and expiring contexts
type contextResolverBadger struct {
	db *badger.DB
	ticker *time.Ticker
	contextsByKey map[ckey.ContextKey]*Context
	keyGenerator  *ckey.KeyGenerator
	// buffer slice allocated once per contextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	tagsBuffer *util.TagsBuilder
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolverBadger) generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	return cr.keyGenerator.Generate(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.tagsBuffer)
}

func (cr *contextResolverBadger) serializeContextKey(key ckey.ContextKey) []byte {
	return key.ToBytes()
}

// TODO: we probably want to encode it manually to be a bit more efficient here.
func (cr *contextResolverBadger) serializeContext(c *Context) []byte {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	enc.Encode(*c)
	return buffer.Bytes()
}

func (cr *contextResolverBadger) deserializeContext(b []byte) *Context {
	buffer := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buffer)
	c := &Context{}
	dec.Decode(&c)
	return c
}

func newContextResolverBadger() *contextResolverBadger {
	// TODO: Also try ondisk with the proper options, this would reduce memory usage at almost
	//   no cost since we can disable fsync()
	opt := badger.DefaultOptions("").WithInMemory(true)
	db, err := badger.Open(opt)
	if err != nil {
		log.Fatal(err)
	}
	ticker := time.NewTicker(1 * time.Minute)

	cr := &contextResolverBadger{
		db: db,
		ticker: ticker,
		contextsByKey: make(map[ckey.ContextKey]*Context),
		keyGenerator:  ckey.NewKeyGenerator(),
		tagsBuffer:    util.NewTagsBuilder(),
	}
	go cr.runGC()
	return cr
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolverBadger) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
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

func (cr *contextResolverBadger) get(key ckey.ContextKey) (*Context, bool) {
	var context *Context = nil

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

func (cr *contextResolverBadger) length() int {
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

func (cr *contextResolverBadger) close() {
	cr.ticker.Stop()
	err := cr.db.Close()
	if err != nil {
		log.Fatal(err)
	}
}
