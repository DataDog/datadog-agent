// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package aggregator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPayloadItem struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func (m *mockPayloadItem) name() string {
	return m.Name
}

func (m *mockPayloadItem) tags() []string {
	return m.Tags
}

func parseMockPayloadItem(data []byte) (items []*mockPayloadItem, err error) {
	items = []*mockPayloadItem{}
	err = json.Unmarshal(data, &items)
	return items, err
}

func generateTestData() (data [][]byte, err error) {
	items := []*mockPayloadItem{
		{
			Name: "totoro",
			Tags: []string{"age:123", "country:jp"},
		},
		{
			Name: "porco rosso",
			Tags: []string{"age:43", "country:it", "role:pilot"},
		},
	}
	jsonData, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return [][]byte{jsonData}, nil
}

func TestCommonAggregator(t *testing.T) {
	t.Run("ContainsPayloadName", func(t *testing.T) {
		data, err := generateTestData()
		require.NoError(t, err)
		agg := newAggregator(parseMockPayloadItem)
		err = agg.UnmarshallPayloads(data)
		assert.NoError(t, err)
		assert.True(t, agg.ContainsPayloadName("totoro"))
		assert.False(t, agg.ContainsPayloadName("ponyo"))
	})

	t.Run("ContainsPayloadNameAndTags", func(t *testing.T) {
		data, err := generateTestData()
		require.NoError(t, err)
		agg := newAggregator(parseMockPayloadItem)
		err = agg.UnmarshallPayloads(data)
		assert.NoError(t, err)
		assert.True(t, agg.ContainsPayloadNameAndTags("totoro", []string{"age:123"}))
		assert.False(t, agg.ContainsPayloadNameAndTags("porco rosso", []string{"country:it", "role:king"}))
		assert.True(t, agg.ContainsPayloadNameAndTags("porco rosso", []string{"country:it", "role:pilot"}))
	})
}
