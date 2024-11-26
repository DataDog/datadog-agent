// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package funcs

import "sync"

type memoizedArgFuncResult[T any] struct {
	result T
	err    error
}

type memoizedArgFunc[K comparable, T any] struct {
	sync.Mutex
	fn      func(K) (T, error)
	results map[K]memoizedArgFuncResult[T]
}

func (mf *memoizedArgFunc[K, T]) do(arg K) (T, error) {
	mf.Lock()
	defer mf.Unlock()

	res, ok := mf.results[arg]
	if !ok {
		val, err := mf.fn(arg)
		res = memoizedArgFuncResult[T]{result: val, err: err}
		mf.results[arg] = res
	}
	return res.result, res.err
}

// MemoizeArg memoizes the result of a function call based on the argument.
//
// fn is only ever called once for each argument, even if it returns an error.
func MemoizeArg[K comparable, T any](fn func(K) (T, error)) func(K) (T, error) {
	return (&memoizedArgFunc[K, T]{fn: fn, results: make(map[K]memoizedArgFuncResult[T])}).do
}
