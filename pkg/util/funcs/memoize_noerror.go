// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package funcs

import "sync"

type memoizedNoErrorFunc[T any] struct {
	once   sync.Once
	fn     func() T
	result T
}

func (mf *memoizedNoErrorFunc[T]) do() T {
	mf.once.Do(func() {
		mf.result = mf.fn()
	})
	return mf.result
}

// MemoizeNoError the result of a function call.
//
// fn is only ever called once
func MemoizeNoError[T any](fn func() T) func() T {
	return (&memoizedNoErrorFunc[T]{fn: fn}).do
}
