// This file is licensed under the MIT License.
//
// Copyright (c) 2017 Nathan Sweet
// Copyright (c) 2018, 2019 Cloudflare
// Copyright (c) 2019 Authors of Cilium
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package funcs

import (
	"sync"
)

type memoizedFunc[T any] struct {
	once   sync.Once
	fn     func() (T, error)
	result T
	err    error
}

func (mf *memoizedFunc[T]) do() (T, error) {
	mf.once.Do(func() {
		mf.result, mf.err = mf.fn()
	})
	return mf.result, mf.err
}

// Memoize the result of a function call.
//
// fn is only ever called once, even if it returns an error.
func Memoize[T any](fn func() (T, error)) func() (T, error) {
	return (&memoizedFunc[T]{fn: fn}).do
}
