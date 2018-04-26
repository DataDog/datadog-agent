// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	cache := NewTemplateCache()
	assert.Len(t, cache.id2digests, 0)
	assert.Len(t, cache.digest2ids, 0)
	assert.Len(t, cache.digest2template, 0)
}

func TestSet(t *testing.T) {
	cache := NewTemplateCache()

	tpl1 := check.Config{ADIdentifiers: []string{"foo", "bar"}}
	err := cache.Set(tpl1)
	require.Nil(t, err)
	// adding again should be no-op
	err = cache.Set(tpl1)
	require.Nil(t, err)

	assert.Len(t, cache.id2digests, 2)
	assert.Len(t, cache.id2digests["foo"], 1)
	assert.Len(t, cache.id2digests["bar"], 1)
	assert.Len(t, cache.digest2ids, 1)
	assert.Len(t, cache.digest2template, 1)

	tpl2 := check.Config{ADIdentifiers: []string{"foo"}}
	err = cache.Set(tpl2)

	require.Nil(t, err)
	assert.Len(t, cache.id2digests, 2)
	assert.Len(t, cache.id2digests["foo"], 2)
	assert.Len(t, cache.id2digests["bar"], 1)
	assert.Len(t, cache.digest2ids, 2)
	assert.Len(t, cache.digest2template, 2)

	// no identifiers at all
	tpl3 := check.Config{ADIdentifiers: []string{}}
	err = cache.Set(tpl3)

	require.NotNil(t, err)
}

func TestDel(t *testing.T) {
	cache := NewTemplateCache()
	tpl := check.Config{ADIdentifiers: []string{"foo", "bar"}}
	err := cache.Set(tpl)
	require.Nil(t, err)

	err = cache.Del(tpl)
	require.Nil(t, err)

	require.Len(t, cache.id2digests, 2)
	assert.Len(t, cache.id2digests["foo"], 0)
	assert.Len(t, cache.id2digests["bar"], 0)
	assert.Len(t, cache.digest2ids, 0)
	assert.Len(t, cache.digest2template, 0)

	// delete for an unknown identifier
	err = cache.Del(check.Config{ADIdentifiers: []string{"baz"}})
	require.NotNil(t, err)
}

func TestGet(t *testing.T) {
	cache := NewTemplateCache()
	tpl := check.Config{ADIdentifiers: []string{"foo", "bar"}}
	cache.Set(tpl)

	ret, err := cache.Get("foo")
	require.Nil(t, err)
	assert.Len(t, ret, 1)
	tpl2 := ret[0]
	assert.True(t, tpl.Equal(&tpl2))

	// id not in cache
	_, err = cache.Get("baz")
	assert.NotNil(t, err)
}

func TestGetUnresolvedTemplates(t *testing.T) {
	cache := NewTemplateCache()
	tpl := check.Config{ADIdentifiers: []string{"foo", "bar"}}
	cache.Set(tpl)
	expected := map[string]check.Config{
		"foo,bar": tpl,
	}

	assert.Equal(t, cache.GetUnresolvedTemplates(), expected)
}
