// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package funcs

type memoizedNoErrorUnsafeFunc[T any] struct {
	fn     func() T
	result T
	done   bool
}

func (mf *memoizedNoErrorUnsafeFunc[T]) do() T {
	if !mf.done {
		mf.result = mf.fn()
		mf.done = true
	}
	return mf.result
}

// MemoizeNoErrorUnsafe the result of a function call.
// This is a thread unsafe version of MemoizeNoError.
//
// fn is only ever called once, but this implementation is not thread-safe.
// Use MemoizeNoError if you need thread safety. Use this if you need to memoize
// a function that only gets called in a single thread, to avoid the overhead of
// the mutex.
func MemoizeNoErrorUnsafe[T any](fn func() T) func() T {
	return (&memoizedNoErrorUnsafeFunc[T]{fn: fn}).do
}
