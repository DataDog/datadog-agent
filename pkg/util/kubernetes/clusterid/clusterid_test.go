// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterid

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestGet(t *testing.T) {
	cache.Cache.Delete(cacheKey())
	t.Cleanup(func() { cache.Cache.Delete(cacheKey()) })

	// too short
	t.Setenv(EnvVar, "foo")
	cid, err := Get()
	assert.Empty(t, cid)
	assert.Error(t, err)
	_, found := Cached()
	assert.False(t, found)

	// too long
	t.Setenv(EnvVar, "d801b2b1-4811-11ea-8618-121d4d0938a44444444")
	cid, err = Get()
	assert.Empty(t, cid)
	assert.Error(t, err)

	// just right, and cached
	testID := "d801b2b1-4811-11ea-8618-121d4d0938a3"
	t.Setenv(EnvVar, testID)
	cid, err = Get()
	assert.Equal(t, testID, cid)
	assert.NoError(t, err)
	cid, found = Cached()
	assert.True(t, found)
	assert.Equal(t, testID, cid)

	t.Setenv(EnvVar, "")
	cid, err = Get()
	assert.Equal(t, testID, cid)
	assert.NoError(t, err)
}
