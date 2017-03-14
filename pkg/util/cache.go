package util

import (
	"time"

	cache "github.com/patrickmn/go-cache"
)

const (
	defaultCacheExpire = 5 * time.Minute
	defaultCachePurge  = 30 * time.Second
	// AgentCachePrefix is the common root to use to prefix all the cache
	// keys for any value regarding the Agent
	AgentCachePrefix = "agent"

	// encapsulate the cache module for easy refactoring

	// NoExpiration maps to go-cache corresponding value
	NoExpiration = cache.NoExpiration
)

// Cache provides an in-memory key:value store similar to memcached
var Cache = cache.New(defaultCacheExpire, defaultCachePurge)
