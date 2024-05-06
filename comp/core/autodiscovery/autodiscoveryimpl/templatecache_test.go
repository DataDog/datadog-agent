// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestNew(t *testing.T) {
	cache := newTemplateCache()
	assert.Len(t, cache.adIDToDigests, 0)
	assert.Len(t, cache.digestToADId, 0)
	assert.Len(t, cache.digestToTemplate, 0)
}

func TestSet(t *testing.T) {
	cache := newTemplateCache()

	tpl1 := integration.Config{ADIdentifiers: []string{"foo", "bar"}}
	err := cache.set(tpl1)
	require.Nil(t, err)
	// adding again should be no-op
	err = cache.set(tpl1)
	require.Nil(t, err)

	assert.Len(t, cache.adIDToDigests, 2)
	assert.Len(t, cache.adIDToDigests["foo"], 1)
	assert.Len(t, cache.adIDToDigests["bar"], 1)
	assert.Len(t, cache.digestToADId, 1)
	assert.Len(t, cache.digestToTemplate, 1)

	tpl2 := integration.Config{ADIdentifiers: []string{"foo"}}
	err = cache.set(tpl2)

	require.Nil(t, err)
	assert.Len(t, cache.adIDToDigests, 2)
	assert.Len(t, cache.adIDToDigests["foo"], 2)
	assert.Len(t, cache.adIDToDigests["bar"], 1)
	assert.Len(t, cache.digestToADId, 2)
	assert.Len(t, cache.digestToTemplate, 2)

	// no identifiers at all
	tpl3 := integration.Config{ADIdentifiers: []string{}}
	err = cache.set(tpl3)

	require.NotNil(t, err)
}

func TestDel(t *testing.T) {
	cache := newTemplateCache()
	tpl := integration.Config{ADIdentifiers: []string{"foo", "bar"}}
	err := cache.set(tpl)
	require.Nil(t, err)
	tpl2 := integration.Config{ADIdentifiers: []string{"foo"}}
	err = cache.set(tpl2)
	require.Nil(t, err)

	err = cache.del(tpl)
	require.Nil(t, err)

	require.Len(t, cache.adIDToDigests, 1)
	assert.Len(t, cache.adIDToDigests["foo"], 1)
	assert.Len(t, cache.digestToADId, 1)
	assert.Len(t, cache.digestToTemplate, 1)

	err = cache.del(tpl2)
	require.Nil(t, err)

	require.Len(t, cache.adIDToDigests, 0)
	assert.Len(t, cache.digestToADId, 0)
	assert.Len(t, cache.digestToTemplate, 0)

	// delete for an unknown identifier
	err = cache.del(integration.Config{ADIdentifiers: []string{"baz"}})
	require.NotNil(t, err)
}

func TestGet(t *testing.T) {
	cache := newTemplateCache()
	tpl := integration.Config{ADIdentifiers: []string{"foo", "bar"}}
	cache.set(tpl)

	ret, err := cache.get("foo")
	require.Nil(t, err)
	assert.Len(t, ret, 1)
	tpl2 := ret[0]
	assert.True(t, tpl.Equal(&tpl2))

	// id not in cache
	_, err = cache.get("baz")
	assert.NotNil(t, err)
}

func TestGetUnresolvedTemplates(t *testing.T) {
	cache := newTemplateCache()
	tpl := integration.Config{ADIdentifiers: []string{"foo", "bar"}}
	cache.set(tpl)
	expected := map[string][]integration.Config{
		"foo,bar": {tpl},
	}

	assert.Equal(t, cache.getUnresolvedTemplates(), expected)
}
