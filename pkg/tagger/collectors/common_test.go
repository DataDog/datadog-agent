// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireMatchInfo(t *testing.T, expected []*TagInfo, item *TagInfo) bool {
	t.Helper()
	for _, template := range expected {
		if template.Entity != item.Entity {
			continue
		}
		if template.Source != item.Source {
			continue
		}
		sort.Strings(template.LowCardTags)
		sort.Strings(item.LowCardTags)
		require.Equal(t, template.LowCardTags, item.LowCardTags)

		sort.Strings(template.HighCardTags)
		sort.Strings(item.HighCardTags)
		require.Equal(t, template.HighCardTags, item.HighCardTags)

		require.Equal(t, template.DeleteEntity, item.DeleteEntity)

		return true
	}

	t.Logf("could not find expected result for entity %s with sourcce %s", item.Entity, item.Source)
	return false
}

func assertTagInfoEqual(t *testing.T, expected *TagInfo, item *TagInfo) bool {
	t.Helper()
	sort.Strings(expected.LowCardTags)
	sort.Strings(item.LowCardTags)

	sort.Strings(expected.OrchestratorCardTags)
	sort.Strings(item.OrchestratorCardTags)

	sort.Strings(expected.HighCardTags)
	sort.Strings(item.HighCardTags)

	sort.Strings(expected.StandardTags)
	sort.Strings(item.StandardTags)

	return assert.Equal(t, expected, item)
}

func assertTagInfoListEqual(t *testing.T, expectedUpdates []*TagInfo, updates []*TagInfo) {
	t.Helper()
	assert.Equal(t, len(expectedUpdates), len(updates))
	for i := 0; i < len(expectedUpdates); i++ {
		assertTagInfoEqual(t, expectedUpdates[i], updates[i])
	}
}

func Test_mergeMaps(t *testing.T) {
	tests := []struct {
		name   string
		first  map[string]string
		second map[string]string
		want   map[string]string
	}{
		{
			name:   "no conflict",
			first:  map[string]string{"first-k1": "first-v1", "first-k2": "first-v2"},
			second: map[string]string{"second-k1": "second-v1", "second-k2": "second-v2"},
			want: map[string]string{
				"first-k1":  "first-v1",
				"first-k2":  "first-v2",
				"second-k1": "second-v1",
				"second-k2": "second-v2",
			},
		},
		{
			name:   "conflict",
			first:  map[string]string{"first-k1": "first-v1", "first-k2": "first-v2"},
			second: map[string]string{"first-k2": "second-v1", "second-k2": "second-v2"},
			want: map[string]string{
				"first-k1":  "first-v1",
				"first-k2":  "first-v2",
				"second-k2": "second-v2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualValues(t, tt.want, mergeMaps(tt.first, tt.second))
		})
	}
}
