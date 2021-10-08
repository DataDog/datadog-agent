// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"time"

	"github.com/patrickmn/go-cache"
)

// ExpressionCache implements cached parsing for expressions
type ExpressionCache struct {
	cache *cache.Cache
}

// NewCache returns a new instance of evalCache
func NewCache(defaultExpiration, cleanupInterval time.Duration) *ExpressionCache {
	cache := cache.New(defaultExpiration, cleanupInterval)
	return &ExpressionCache{
		cache: cache,
	}
}

func (c *ExpressionCache) cached(key string, fn func() (interface{}, error)) (interface{}, error) {
	if v, ok := c.cache.Get(key); ok {
		return v, nil
	}
	v, err := fn()
	if err != nil {
		return nil, err
	}
	c.cache.Set(key, v, cache.NoExpiration)
	return v, nil
}

// ParseExpression parses Expression from a string
func (c *ExpressionCache) ParseExpression(s string) (*Expression, error) {
	v, err := c.cached(s, func() (interface{}, error) {
		return ParseExpression(s)
	})
	if err != nil {
		return nil, err
	}
	return v.(*Expression), err
}

func iterableCacheKey(s string) string {
	return "iterable:" + s
}

// ParseIterable parses IterableExpression from a string
func (c *ExpressionCache) ParseIterable(s string) (*IterableExpression, error) {
	v, err := c.cached(iterableCacheKey(s), func() (interface{}, error) {
		return ParseIterable(s)
	})
	if err != nil {
		return nil, err
	}
	return v.(*IterableExpression), err
}

func pathCacheKey(s string) string {
	return "path:" + s
}

// ParsePath parses PathExpression from a string
func (c *ExpressionCache) ParsePath(s string) (*PathExpression, error) {
	v, err := c.cached(pathCacheKey(s), func() (interface{}, error) {
		return ParsePath(s)
	})
	if err != nil {
		return nil, err
	}
	return v.(*PathExpression), err
}

// Cache declares a default cache for expression evaluation with no key experation
var Cache = NewCache(cache.NoExpiration, 0)
