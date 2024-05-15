package cache

import "github.com/DataDog/datadog-agent/pkg/persistentcache"

type Cacher interface {
	Read(key string) (string, error)
	Write(key string, value string) error
}

type PersistentCacher struct {
}

func (p *PersistentCacher) Read(key string) (string, error) {
	return persistentcache.Read(key)
}

func (p *PersistentCacher) Write(key string, value string) error {
	return persistentcache.Write(key, value)
}
