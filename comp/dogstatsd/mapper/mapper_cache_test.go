// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mapper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapperCache(t *testing.T) {
	c, err := newMapperCache(10)
	assert.NoError(t, err)

	assert.Equal(t, 0, c.cache.Len())

	c.add("metric_name", &MapResult{Name: "mapped_name", Tags: []string{"foo", "bar"}, matched: true})
	c.add("metric_name2", &MapResult{Name: "mapped_name", Tags: []string{"foo", "bar"}, matched: true})
	c.add("metric_name3", &MapResult{Name: "mapped_name", Tags: []string{"foo", "bar"}, matched: true})
	c.add("metric_miss1", &MapResult{matched: false})
	c.add("metric_miss2", &MapResult{matched: false})
	assert.Equal(t, 5, c.cache.Len())

	result, found := c.get("metric_name")
	assert.Equal(t, true, found)
	assert.Equal(t, &MapResult{Name: "mapped_name", matched: true, Tags: []string{"foo", "bar"}}, result)

	result, found = c.get("metric_name_not_exist")
	assert.Equal(t, false, found)
	assert.Equal(t, (*MapResult)(nil), result)

	result, found = c.get("metric_miss1")
	assert.Equal(t, true, found)
	assert.Equal(t, &MapResult{matched: false}, result)
}
