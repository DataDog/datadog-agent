// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"path"
	"time"

	cache "github.com/patrickmn/go-cache"
)

const (
	defaultExpire = 5 * time.Minute
	defaultPurge  = 30 * time.Second
	// AgentCachePrefix is the common root to use to prefix all the cache
	// keys for any value regarding the Agent
	AgentCachePrefix = "agent"

	// encapsulate the cache module for easy refactoring

	// NoExpiration maps to go-cache corresponding value
	NoExpiration = cache.NoExpiration
)

// Cache provides an in-memory key:value store similar to memcached
var Cache = cache.New(defaultExpire, defaultPurge)

// BuildAgentKey creates a cache key by joining the constant AgentCachePrefix
// and path elements passed as arguments. It is to be used by core agent
// packages to reuse the prefix constant
func BuildAgentKey(keys ...string) string {
	keys = append([]string{AgentCachePrefix}, keys...)
	return path.Join(keys...)
}

// Get returns the value for 'key'.
//
// cache hit:
//
//	pull the value from the cache and returns it.
//
// cache miss:
//
//	call 'cb' function to get a new value. If the callback doesn't return an error the returned value is
//	cached with no expiration date and returned.
func Get[T any](key string, cb func() (T, error)) (T, error) {
	return GetWithExpiration[T](key, cb, cache.NoExpiration)
}

// GetWithExpiration returns the value for 'key'.
//
// cache hit:
//
//	pull the value from the cache and returns it.
//
// cache miss:
//
//	call 'cb' function to get a new value. If the callback doesn't return an error the returned value is
//	cached with the given expire duration and returned.
func GetWithExpiration[T any](key string, cb func() (T, error), expire time.Duration) (T, error) {
	if x, found := Cache.Get(key); found {
		return x.(T), nil
	}

	res, err := cb()
	// We don't cache errors
	if err == nil {
		Cache.Set(key, res, expire)
	}
	return res, err
}
