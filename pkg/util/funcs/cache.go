// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package funcs provides utilities for functions, such as caching and memoization.
package funcs

import "sync"

// CachedFunc represents a function which caches its result, but can also be flushed.
type CachedFunc[T any] interface {
	Do() (*T, error)
	Flush()
}

type cachedFunc[T any] struct {
	mtx    sync.RWMutex
	fn     func() (*T, error)
	cb     func()
	result *T
}

// Do either returns a cached result, or executes the stored function.
func (mf *cachedFunc[T]) Do() (*T, error) {
	mf.mtx.RLock()
	res := mf.result
	mf.mtx.RUnlock()

	if res != nil {
		return res, nil
	}

	mf.mtx.Lock()
	defer mf.mtx.Unlock()

	var err error
	res, err = mf.fn()
	if err != nil {
		return nil, err
	}

	mf.result = res
	return res, nil
}

// Flush deletes the stored result, ensuring the next call to [Do] will execute the function.
func (mf *cachedFunc[T]) Flush() {
	mf.mtx.Lock()
	defer mf.mtx.Unlock()

	mf.result = nil
	if mf.cb != nil {
		mf.cb()
	}
}

// Cache the result of a function call, with the ability to flush the cache.
func Cache[T any](fn func() (*T, error)) CachedFunc[T] {
	return &cachedFunc[T]{fn: fn}
}

// CacheWithCallback the result of a function call, with the ability to flush the cache.
// The provided callback function will be called when the cache is flushed.
func CacheWithCallback[T any](fn func() (*T, error), cb func()) CachedFunc[T] {
	return &cachedFunc[T]{fn: fn, cb: cb}
}
