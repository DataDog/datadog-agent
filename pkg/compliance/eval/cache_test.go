// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"

	"github.com/patrickmn/go-cache"
	assert "github.com/stretchr/testify/require"
)

func newTestCache() *ExpressionCache {
	return NewCache(cache.NoExpiration, 0)
}
func TestCacheParseExpression(t *testing.T) {
	assert := assert.New(t)

	const s = "a > 5"
	cache := newTestCache()
	expr, err := cache.ParseExpression(s)

	assert.NotNil(expr)
	assert.NoError(err)

	cached, ok := cache.cache.Get(s)
	assert.Equal(cached, expr)
	assert.True(ok)
}

func TestCacheParseExpressionError(t *testing.T) {
	assert := assert.New(t)

	cache := newTestCache()
	expr, err := cache.ParseExpression("~")

	assert.Nil(expr)
	assert.EqualError(err, `1:1: unexpected token "~"`)
	assert.Zero(cache.cache.ItemCount())
}

func TestCacheParseIterable(t *testing.T) {
	assert := assert.New(t)

	const s = "count(a > 5) > 0"

	cache := newTestCache()
	expr, err := cache.ParseIterable(s)

	assert.NotNil(expr)
	assert.NoError(err)

	cached, ok := cache.cache.Get(iterableCacheKey(s))
	assert.Equal(cached, expr)
	assert.True(ok)
}

func TestCacheParseIterableError(t *testing.T) {
	assert := assert.New(t)

	cache := newTestCache()
	expr, err := cache.ParseIterable("len(5 >)")

	assert.Nil(expr)
	assert.EqualError(err, `1:7: unexpected token ">" (expected ")")`)
	assert.Zero(cache.cache.ItemCount())
}

func TestCacheParsePath(t *testing.T) {
	assert := assert.New(t)

	const s = "/etc/bitsy/spider"

	cache := newTestCache()
	expr, err := cache.ParsePath(s)

	assert.NotNil(expr)
	assert.NoError(err)

	cached, ok := cache.cache.Get(pathCacheKey(s))
	assert.Equal(cached, expr)
	assert.True(ok)
}

func TestCacheParsePathError(t *testing.T) {
	assert := assert.New(t)

	cache := newTestCache()
	expr, err := cache.ParsePath(`=/abc/`)

	assert.Nil(expr)
	assert.EqualError(err, `1:1: unexpected token "="`)
	assert.Zero(cache.cache.ItemCount())
}
