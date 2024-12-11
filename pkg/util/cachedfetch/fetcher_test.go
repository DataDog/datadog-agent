// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cachedfetch

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// If Attempt never succeeds, f.Fetch returns an error
func TestFetcherNeverSucceeds(t *testing.T) {
	f := Fetcher{
		Attempt: func(context.Context) (interface{}, error) { return nil, fmt.Errorf("uhoh") },
	}

	v, err := f.Fetch(context.TODO())
	require.Nil(t, v)
	require.Error(t, err)

	v, err = f.Fetch(context.TODO())
	require.Nil(t, v)
	require.Error(t, err)
}

// Each call to f.Fetch() calls Attempt again
func TestFetcherCalledEachFetch(t *testing.T) {
	count := 0
	f := Fetcher{
		Attempt: func(context.Context) (interface{}, error) {
			count++
			return count, nil
		},
	}

	v, err := f.Fetch(context.TODO())
	require.Equal(t, 1, v)
	require.NoError(t, err)

	v, err = f.Fetch(context.TODO())
	require.Equal(t, 2, v)
	require.NoError(t, err)
}

// After a successful call, f.Fetch does not fail
func TestFetcherUsesCachedValue(t *testing.T) {
	count := 0
	f := Fetcher{
		Name: "test",
		Attempt: func(context.Context) (interface{}, error) {
			count++
			if count%2 == 0 {
				return nil, fmt.Errorf("uhoh")
			}
			return count, nil
		},
	}

	for iter, exp := range []int{1, 1, 3, 3, 5, 5} {
		v, err := f.Fetch(context.TODO())
		require.Equal(t, exp, v, "on iteration %d", iter)
		require.NoError(t, err)
	}
}

// Errors are logged with LogFailure
func TestFetcherLogsWhenUsingCached(t *testing.T) {
	count := 0
	errs := []string{}
	f := Fetcher{
		Attempt: func(context.Context) (interface{}, error) {
			count++
			if count%2 == 0 {
				return nil, fmt.Errorf("uhoh")
			}
			return count, nil
		},
		LogFailure: func(err error, v interface{}) {
			errs = append(errs, fmt.Sprintf("%v, %v", err, v))
		},
	}

	for iter, exp := range []int{1, 1, 3, 3} {
		v, err := f.Fetch(context.TODO())
		require.Equal(t, exp, v, "on iteration %d", iter)
		require.NoError(t, err)
	}

	require.Equal(t, []string{"uhoh, 1", "uhoh, 3"}, errs)
}

// FetchString casts to a string
func TestFetchString(t *testing.T) {
	f := Fetcher{
		Attempt: func(context.Context) (interface{}, error) { return "hello", nil },
	}
	v, err := f.FetchString(context.TODO())
	require.Equal(t, "hello", v)
	require.NoError(t, err)
}

// FetchString casts to a string
func TestFetchStringError(t *testing.T) {
	f := Fetcher{
		Attempt: func(context.Context) (interface{}, error) { return nil, fmt.Errorf("uhoh") },
	}
	v, err := f.FetchString(context.TODO())
	require.Equal(t, "", v)
	require.Error(t, err)
}

// FetchStringSlice casts to a []string
func TestFetchStringSlice(t *testing.T) {
	f := Fetcher{
		Attempt: func(context.Context) (interface{}, error) { return []string{"hello"}, nil },
	}
	v, err := f.FetchStringSlice(context.TODO())
	require.Equal(t, []string{"hello"}, v)
	require.NoError(t, err)
}

// FetchStringSlice casts to a []string
func TestFetchStringSliceError(t *testing.T) {
	f := Fetcher{
		Attempt: func(context.Context) (interface{}, error) { return nil, fmt.Errorf("uhoh") },
	}
	v, err := f.FetchStringSlice(context.TODO())
	require.Nil(t, v)
	require.Error(t, err)
}

func TestReset(t *testing.T) {
	succeed := func(context.Context) (interface{}, error) { return "yay", nil }
	fail := func(context.Context) (interface{}, error) { return nil, fmt.Errorf("uhoh") }
	f := Fetcher{}

	f.Attempt = succeed
	v, err := f.FetchString(context.TODO())
	require.Equal(t, "yay", v)
	require.NoError(t, err)

	f.Attempt = fail
	v, err = f.FetchString(context.TODO())
	require.Equal(t, "yay", v)
	require.NoError(t, err)

	f.Reset()

	v, err = f.FetchString(context.TODO())
	require.Equal(t, "", v)
	require.Error(t, err)
}
