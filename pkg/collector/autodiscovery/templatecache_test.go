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
}
