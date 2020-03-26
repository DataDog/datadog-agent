// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package collectors

import (
	"fmt"
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

	sort.Strings(expected.OrchestratorCardTags)
	sort.Strings(item.OrchestratorCardTags)

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

func TestResolveTag(t *testing.T) {
	testCases := []struct {
		tmpl, label, expected string
	}{
		{
			"kube_%%label%%", "app", "kube_app",
		},
		{
			"foo_%%label%%_bar", "app", "foo_app_bar",
		},
		{
			"%%label%%%%label%%", "app", "appapp",
		},
		{
			"kube_", "app", "kube_", // no template variable
		},
		{
			"kube_%%foo%%", "app", "kube_", // unsupported template variable
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			tagName := resolveTag(testCase.tmpl, testCase.label)
			assert.Equal(t, testCase.expected, tagName)
		})
	}
}
