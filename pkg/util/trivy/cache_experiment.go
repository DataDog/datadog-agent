package trivy

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"k8s.io/apimachinery/pkg/util/errors"
)

var (
	ttlTicker = 5 * time.Minute
)

type PersistentDatabase interface {
	Store(string, []byte) error
	Get(string) ([]byte, error)
	Delete(string) error
	GetAllKeys() []string
}

type PersistentCache struct {
	ttlCache *simplelru.LRU[string, time.Time]
	db       PersistentDatabase
	ttl      *time.Duration
	ticker   *time.Ticker
}

func NewPersistentCache(
	cacheSize int,
	localDB PersistentDatabase,
	ttl time.Duration,
) (*PersistentCache, error) {

	onEvict := func(k string, _ time.Time) {
		if err := localDB.Delete(k); err != nil {
			log.Errorf("could not delete [%s] from cache: %v", k, err)
		}
	}

	ttlCache, err := simplelru.NewLRU[string, time.Time](cacheSize, onEvict)
	if err != nil {
		return nil, err
	}

	ttlTicker := time.NewTicker(ttlTicker)
	persistentCache := &PersistentCache{
		ttlCache: ttlCache,
		db:       localDB,
		ttl:      &ttl,
		ticker:   ttlTicker,
	}

	go func() {
		for {
			select {
			case <-ttlTicker.C:
				persistentCache.CleanExpiredEntries()
			}
		}
	}()

	return persistentCache, nil
}

func (c *PersistentCache) CleanExpiredEntries() {
	for _, key := range c.ttlCache.Keys() {
		expiresAt, ok := c.ttlCache.Get(key)
		if !ok {
			continue
		}
		if expiresAt.After(time.Now()) {
			_ = c.ttlCache.Remove(key)
		}
	}
}

func (c *PersistentCache) Store(key string, value []byte) error {
	if err := c.db.Store(key, value); err != nil {
		_ = c.ttlCache.Remove(key)
		return err
	}

	var expiration time.Time

	if c.ttl != nil {
		expiration = time.Now().Add(*c.ttl)
	} else {
		expiration = time.Unix(1<<63-1, 0)
	}

	_ = c.ttlCache.Add(key, expiration)
	return nil
}

func (c *PersistentCache) Get(key string) ([]byte, error) {
	res, err := c.db.Get(key)
	if err != nil {
		_ = c.ttlCache.Remove(key)
		return nil, err
	}

	_, ok := c.ttlCache.Get(key)
	if !ok {
		_ = c.db.Delete(key)
		return nil, fmt.Errorf("entry [%s] exists in database but not in the cache", key)
	}

	if c.ttl != nil {
		c.ttlCache.Add(key, time.Now().Add(*c.ttl))
	}

	return res, nil
}

func (c *PersistentCache) Delete(key string) error {
	ok := c.ttlCache.Remove(key)
	var err error
	if !ok {
		err = fmt.Errorf("entry [%s] does not exist in the cache", key)
	}
	return errors.NewAggregate([]error{
		err,
		c.db.Delete(key),
	})
}
