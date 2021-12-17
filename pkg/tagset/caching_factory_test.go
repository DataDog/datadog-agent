// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCachingFactory(t *testing.T) {
	testFactory(t, func() Factory {
		f, _ := NewCachingFactory(10, 5)
		return f
	})
	testFactoryCaching(t, func() Factory {
		f, _ := NewCachingFactory(10, 5)
		return f
	})
}

func TestCachingFactory_Union_Fuzz(t *testing.T) {
	f, _ := NewCachingFactory(100, 1)
	fuzz(t, func(seed int64) {
		r := rand.New(rand.NewSource(seed))

		bothBuilder := f.NewBuilder(30)

		n := r.Intn(15)
		aBuilder := f.NewBuilder(n)
		for i := 0; i < n; i++ {
			t := fmt.Sprintf("tag%d", r.Intn(30))
			aBuilder.Add(t)
			bothBuilder.Add(t)
		}
		a := aBuilder.Close()

		n = r.Intn(15)
		bBuilder := f.NewBuilder(n)
		for i := 0; i < n; i++ {
			t := fmt.Sprintf("tag%d", r.Intn(30))
			bBuilder.Add(t)
			bothBuilder.Add(t)
		}
		b := bBuilder.Close()

		union := f.Union(a, b)
		union.validate(t)

		both := bothBuilder.Close()
		both.validate(t)

		require.Equal(t, both.Hash(), union.Hash())
		require.Equal(t, both.Sorted(), union.Sorted())
	})
}
