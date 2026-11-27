// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package memory

import (
	"context"
	"sync"
	"testing"

	"github.com/netsampler/goflow2/decoders/netflow/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func key(domain uint32, id uint16) *templates.TemplateKey {
	return templates.NewTemplateKey("10.0.0.1:2055", 10, domain, id)
}

func get(t *testing.T, d *Driver, k *templates.TemplateKey) interface{} {
	t.Helper()
	v, err := d.GetTemplate(t.Context(), k)
	require.NoError(t, err)
	return v
}

func TestSizeNeverExceedsMax(t *testing.T) {
	ctx := t.Context()
	d := New(100)
	for dom := uint32(0); dom < 50; dom++ {
		for id := uint16(256); id < 356; id++ {
			require.NoError(t, d.AddTemplate(ctx, key(dom, id), "tmpl"))
			require.LessOrEqual(t, d.Len(), 100)
		}
	}
	assert.Equal(t, 100, d.Len())
	assert.Equal(t, uint64(5000-100), d.Evictions())
}

func TestRepeatedKeyUpdatesInPlace(t *testing.T) {
	ctx := t.Context()
	d := New(2)
	require.NoError(t, d.AddTemplate(ctx, key(1, 256), "a"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 256), "b"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 256), "c"))
	assert.Equal(t, 1, d.Len())
	assert.Zero(t, d.Evictions())
	assert.Equal(t, "c", get(t, d, key(1, 256)))
}

func TestLRUEviction(t *testing.T) {
	ctx := t.Context()
	d := New(2)
	require.NoError(t, d.AddTemplate(ctx, key(1, 1), "one"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 2), "two"))
	// Reading key 1 makes key 2 the least recently used.
	assert.Equal(t, "one", get(t, d, key(1, 1)))
	require.NoError(t, d.AddTemplate(ctx, key(1, 3), "three"))

	assert.Equal(t, "one", get(t, d, key(1, 1)))
	assert.Nil(t, get(t, d, key(1, 2)))
	assert.Equal(t, "three", get(t, d, key(1, 3)))
}

func TestUpdateRefreshesRecency(t *testing.T) {
	ctx := t.Context()
	d := New(2)
	require.NoError(t, d.AddTemplate(ctx, key(1, 1), "one"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 2), "two"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 1), "one-v2"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 3), "three"))

	assert.Equal(t, "one-v2", get(t, d, key(1, 1)))
	assert.Nil(t, get(t, d, key(1, 2)))
}

func TestUnlimited(t *testing.T) {
	ctx := t.Context()
	d := New(0)
	for id := uint16(0); id < 1000; id++ {
		require.NoError(t, d.AddTemplate(ctx, key(1, id), id))
	}
	assert.Equal(t, 1000, d.Len())
	assert.Zero(t, d.Evictions())
}

func TestGetMissing(t *testing.T) {
	assert.Nil(t, get(t, New(1), key(9, 9)))
}

func TestListTemplates(t *testing.T) {
	ctx := t.Context()
	d := New(2)
	require.NoError(t, d.AddTemplate(ctx, key(1, 1), "one"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 2), "two"))
	require.NoError(t, d.AddTemplate(ctx, key(1, 3), "three"))

	ch := make(chan *templates.TemplateKey, 10)
	require.NoError(t, d.ListTemplates(ctx, ch))
	var got []templates.TemplateKey
	for k := <-ch; k != nil; k = <-ch {
		got = append(got, *k)
	}
	assert.ElementsMatch(t, []templates.TemplateKey{*key(1, 2), *key(1, 3)}, got)
}

func TestListTemplatesContextCancelled(t *testing.T) {
	d := New(0)
	require.NoError(t, d.AddTemplate(t.Context(), key(1, 1), "one"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, d.ListTemplates(ctx, make(chan *templates.TemplateKey)), context.Canceled)
}

func TestConcurrentAccess(t *testing.T) {
	ctx := t.Context()
	d := New(50)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				k := key(uint32(g), uint16(i))
				_ = d.AddTemplate(ctx, k, i)
				_, _ = d.GetTemplate(ctx, k)
			}
		}(g)
	}
	wg.Wait()
	assert.LessOrEqual(t, d.Len(), 50)
}
