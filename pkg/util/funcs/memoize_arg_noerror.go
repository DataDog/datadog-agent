// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package funcs

import "sync"

type memoizedArgNoErrorFunc[K comparable, T any] struct {
	sync.Mutex
	fn      func(K) T
	results map[K]T
}

func (mf *memoizedArgNoErrorFunc[K, T]) do(arg K) T {
	mf.Lock()
	defer mf.Unlock()

	res, ok := mf.results[arg]
	if !ok {
		res = mf.fn(arg)
		mf.results[arg] = res
	}
	return res
}

// MemoizeArgNoError memoizes the result of a function call based on the argument.
//
// fn is only ever called once for each argument
func MemoizeArgNoError[K comparable, T any](fn func(K) T) func(K) T {
	return (&memoizedArgNoErrorFunc[K, T]{fn: fn, results: make(map[K]T)}).do
}
