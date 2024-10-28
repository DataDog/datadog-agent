// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package buf

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferedChan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewBufferedChan(ctx, 10, 3)
	go func() {
		for n := 0; n < 10000; n++ {
			require.True(t, c.Put(n))
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

	// `Put`` must return false as the channel is canceled.
	for ok := c.Put(0); ok == true; {
		ok = c.Put(0)
	}
}

func TestBufferedChanThreadSafety(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewBufferedChan(ctx, 10, 3)
	go func() {
		random := rand.New(rand.NewSource(42))
		for n := 0; n < 3000; n++ {
			require.True(t, c.Put(n))
			time.Sleep(time.Microsecond * time.Duration(random.Int31n(1000)))
		}
		c.Close()
	}()

	r := require.New(t)
	n := 0
	random := rand.New(rand.NewSource(42))
	for v, ok := c.Get(); ok; v, ok = c.Get() {
		r.Equal(n, v)
		time.Sleep(time.Microsecond * time.Duration(random.Int31n(1000)))
		n++
	}
	r.Equal(3000, n)
}

func TestBufferedChanWaitForValueTrue(t *testing.T) {
	bufferSize := 3
	c := NewBufferedChan(context.Background(), 10, bufferSize)

	go func() {
		for i := 0; i <= bufferSize; i++ {
			c.Put(42)
		}
	}()
	assert.True(t, c.WaitForValue())
}

func TestBufferedChanWaitForValueFalse(t *testing.T) {
	c := NewBufferedChan(context.Background(), 10, 3)

	go func() {
		c.Close()
	}()
	assert.False(t, c.WaitForValue())
}
