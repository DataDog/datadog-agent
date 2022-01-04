// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBufferedChan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewBufferedChan(ctx, 10, 3)
	go func() {
		for n := 0; n < 10000; n++ {
			require.NoError(t, c.Put(n))
		}
		c.Close()
	}()

	r := require.New(t)
	n := 0
	for v, ok := c.Get(); ok; v, ok = c.Get() {
		r.Equal(n, v)
		n++
	}
	r.Equal(10000, n)
}

func TestBufferedChanContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := NewBufferedChan(ctx, 1, 1)
	go func() {
		cancel()
	}()

	// no timeout
	v, found := c.Get()
	r := require.New(t)
	r.False(found)
	r.Nil(v)

	// `Put`` must return an error as the channel is canceled.
	for err := c.Put(0); err == nil; err = c.Put(0) {
	}
}
