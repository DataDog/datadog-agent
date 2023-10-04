// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cache implements a cache
package cache

import (
	"strings"
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
	// NOTE this function is called repeatedly in the normal operation of
	// the Agent. In PR #19125 we modified its internals to allocate
	// less. Please be aware that this function is an allocation hotspot.
	var builder strings.Builder

	// Preallocate memory for the passed keys and the slashes. This will be
	// one character too high as we do not trail keys with a '/' but the
	// logic here is simple.
	totalLength := len(AgentCachePrefix)
	for i := 0; i < len(keys); i++ {
		totalLength += len(keys[i]) + 1
	}

	builder.Grow(totalLength)

	// If we have no keys passed the return is simply "agent". Else, we
	// always insert a '/' between keys and then the key itself. Previous
	// implementations used `join.Path` to insert '/' but we never have a
	// blank string as first member and we never have '../..' etc present in
	// the keys.
	builder.WriteString(AgentCachePrefix)
	for i := 0; i < len(keys); i++ {
		builder.WriteByte('/')
		builder.WriteString(keys[i])
	}

	return builder.String()
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
