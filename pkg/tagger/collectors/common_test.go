package collectors

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireMatchInfo(t *testing.T, expected []*TagInfo, item *TagInfo) bool {
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
	sort.Strings(expected.LowCardTags)
	sort.Strings(item.LowCardTags)

	sort.Strings(expected.HighCardTags)
	sort.Strings(item.HighCardTags)

	return assert.Equal(t, expected, item)
}

func assertTagInfoListEqual(t *testing.T, expectedUpdates []*TagInfo, updates []*TagInfo) {
	assert.Equal(t, len(expectedUpdates), len(updates))
	for i := 0; i < len(expectedUpdates); i++ {
		assertTagInfoEqual(t, expectedUpdates[i], updates[i])
	}
}
