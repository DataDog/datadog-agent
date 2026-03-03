// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package testutil

import (
	"fmt"
	"iter"
	"runtime"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ElementsMatchFn checks that for each element there is exactly one
// matcher function that does not fail. Order of elements and matchers
// does not matter.
//
// Using require instead of assert in the matchers will produce more
// compact and readable failures.
//
// seq can be obtained from slices.All() or maps.All().
func ElementsMatchFn[K comparable, V any](
	t assert.TestingT,
	seq iter.Seq2[K, V],
	matchers ...func(require.TestingT, K, V),
) {
	used := make([]bool, len(matchers))
	errs := make([]map[K][]error, len(matchers))
	miss := map[K]V{}
	for k, v := range seq {
		ok := false
		for i, fn := range matchers {
			if used[i] {
				continue
			}
			if errs[i] == nil {
				errs[i] = make(map[K][]error)
			}
			errs[i][k] = tryCollect(fn, k, v)
			if len(errs[i][k]) == 0 {
				ok = true
				used[i] = true
				errs[i] = nil
				break
			}
		}
		if !ok {
			miss[k] = v
		}
	}
	for i, matcherErrs := range errs {
		for k, keyErrs := range matcherErrs {
			assert.Fail(t, fmt.Sprintf("matcher #%d did not match element %#v", i, k), keyErrs)
			// skip less detailed reports for this element and matcher
			delete(miss, k)
			used[i] = true
		}
	}
	for k, v := range miss {
		assert.Fail(t, fmt.Sprintf("element %#v did not match anything", k), v)
	}
	for i, flag := range used {
		if !flag {
			assert.Fail(t, fmt.Sprintf("matcher %d did not match anything", i))
		}
	}
}

// collectT is a minimal implementation of require.TestingT
type collectT []error

func (c *collectT) Errorf(f string, args ...any) {
	*c = append(*c, fmt.Errorf(f, args...))
}

func (c *collectT) FailNow() {
	runtime.Goexit()
}

func tryCollect[K, V any](fn func(c require.TestingT, k K, v V), k K, v V) []error {
	c := collectT{}
	w := make(chan struct{})
	go func() {
		defer close(w)
		fn(&c, k, v)
	}()
	<-w
	return c
}
