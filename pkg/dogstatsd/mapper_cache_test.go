package dogstatsd

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMapperCache(t *testing.T) {
	c, err := newMapperCache(10)
	assert.NoError(t, err)

	assert.Equal(t, 0, c.cache.Len())

	c.addMatch("metric_name", "mapped_name", []string{"foo", "bar"})
	c.addMatch("metric_name2", "mapped_name", []string{"foo", "bar"})
	c.addMatch("metric_name3", "mapped_name", []string{"foo", "bar"})
	c.addMiss("metric_miss1")
	c.addMiss("metric_miss2")
	assert.Equal(t, 5, c.cache.Len())

	result, found := c.get("metric_name")
	assert.Equal(t, true, found)
	assert.Equal(t, &mapperCacheResult{Name: "mapped_name", Matched: true, Tags: []string{"foo", "bar"}}, result)

	result, found = c.get("metric_name_not_exist")
	assert.Equal(t, false, found)
	assert.Equal(t, (*mapperCacheResult)(nil), result)

	result, found = c.get("metric_miss1")
	assert.Equal(t, true, found)
	assert.Equal(t, &mapperCacheResult{Matched: false}, result)
}
