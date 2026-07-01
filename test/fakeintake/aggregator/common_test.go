// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package aggregator

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPayloadItem struct {
	collectedTime time.Time
	Name          string   `json:"name"`
	Tags          []string `json:"tags"`
}

func (m *mockPayloadItem) name() string {
	return m.Name
}

func (m *mockPayloadItem) GetTags() []string {
	return m.Tags
}

func (m *mockPayloadItem) GetCollectedTime() time.Time {
	return m.collectedTime
}

func parseMockPayloadItem(payload api.Payload) (items []*mockPayloadItem, err error) {
	items = []*mockPayloadItem{}
	err = json.Unmarshal(payload.Data, &items)
	for _, i := range items {
		i.collectedTime = payload.Timestamp
	}
	return items, err
}

func generateTestData() (data []api.Payload, err error) {
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
	return []api.Payload{
		{
			Data:      jsonData,
			Timestamp: time.Now(),
		},
	}, nil
}

func validateCollectionTime(t *testing.T, agg *Aggregator[*mockPayloadItem]) {
	if runtime.GOOS != "linux" {
		t.Logf("validateCollectionTime test skip on %s", runtime.GOOS)
		return
	}
	for _, n := range agg.GetNames() {
		for _, p := range agg.GetPayloadsByName(n) {
			assert.True(t, p.GetCollectedTime().Before(time.Now()), "collection time not in the past %v %v", p.GetCollectedTime(), time.Now())
		}
	}
}

func TestCommonAggregator(t *testing.T) {
	t.Run("ContainsPayloadName", func(t *testing.T) {
		agg := newAggregator(parseMockPayloadItem)
		assert.False(t, agg.ContainsPayloadName("totoro"))
		data, err := generateTestData()
		require.NoError(t, err)
		err = agg.UnmarshallPayloads(data)
		assert.NoError(t, err)
		assert.True(t, agg.ContainsPayloadName("totoro"))
		assert.False(t, agg.ContainsPayloadName("ponyo"))
		validateCollectionTime(t, &agg)
	})

	t.Run("ContainsPayloadNameAndTags", func(t *testing.T) {
		agg := newAggregator(parseMockPayloadItem)
		assert.False(t, agg.ContainsPayloadNameAndTags("totoro", []string{"age:123"}))
		data, err := generateTestData()
		require.NoError(t, err)
		err = agg.UnmarshallPayloads(data)
		assert.NoError(t, err)
		assert.True(t, agg.ContainsPayloadNameAndTags("totoro", []string{"age:123"}))
		assert.False(t, agg.ContainsPayloadNameAndTags("porco rosso", []string{"country:it", "role:king"}))
		assert.True(t, agg.ContainsPayloadNameAndTags("porco rosso", []string{"country:it", "role:pilot"}))
		validateCollectionTime(t, &agg)
	})

	t.Run("AreTagsSubsetOfOtherTags", func(t *testing.T) {
		assert.True(t, AreTagsSubsetOfOtherTags([]string{"interface:lo"}, []string{"interface:lo", "snmp_profile:generic-router"}))
		assert.False(t, AreTagsSubsetOfOtherTags([]string{"totoro"}, []string{"interface:lo", "snmp_profile:generic-router"}))
		assert.False(t, AreTagsSubsetOfOtherTags([]string{"totoro", "interface:lo"}, []string{"interface:lo", "snmp_profile:generic-router"}))
	})

	t.Run("FilterByTags", func(t *testing.T) {
		items := []*mockPayloadItem{
			{
				Name: "totoro",
				Tags: []string{"age:123", "country:jp"},
			},
			{
				Name: "totoro",
				Tags: []string{"age:43", "country:jp"},
			},
		}

		assert.NotEmpty(t, FilterByTags(items, []string{"age:123"}))
		assert.NotEmpty(t, FilterByTags(items, []string{"age:123", "country:jp"}))
		assert.Empty(t, FilterByTags(items, []string{"age:123", "country:it"}))
		assert.NotEmpty(t, FilterByTags(items, []string{"age:43"}))
	})

	t.Run("Reset", func(t *testing.T) {
		data, err := generateTestData()
		require.NoError(t, err)
		agg := newAggregator(parseMockPayloadItem)
		err = agg.UnmarshallPayloads(data)
		require.NoError(t, err)
		assert.NotEmpty(t, agg.payloadsByName)
		agg.Reset()
		assert.Empty(t, agg.payloadsByName)
	})

	t.Run("Thread safe", func(t *testing.T) {
		var wg sync.WaitGroup
		data, err := generateTestData()
		require.NoError(t, err)
		agg := newAggregator(parseMockPayloadItem)
		// add some data to ensure we have names
		err = agg.UnmarshallPayloads(data)
		assert.NoError(t, err)
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				err := agg.UnmarshallPayloads(data)
				assert.NoError(t, err)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				names := agg.GetNames()
				assert.NotEmpty(t, names)
			}
		}()
		wg.Wait()
	})

	t.Run("UnmarshallPayloads merges instead of replacing", func(t *testing.T) {
		// First batch: totoro and porco rosso
		data1, err := generateTestData()
		require.NoError(t, err)

		agg := newAggregator(parseMockPayloadItem)
		err = agg.UnmarshallPayloads(data1)
		require.NoError(t, err)

		// Should have 2 names
		assert.ElementsMatch(t, []string{"totoro", "porco rosso"}, agg.GetNames())
		assert.Len(t, agg.GetPayloadsByName("totoro"), 1)

		// Second batch: a new totoro payload (disjoint from the first)
		items2 := []*mockPayloadItem{
			{Name: "totoro", Tags: []string{"age:999"}},
			{Name: "ponyo", Tags: []string{"age:5"}},
		}
		jsonData2, err := json.Marshal(items2)
		require.NoError(t, err)
		data2 := []api.Payload{{Data: jsonData2, Timestamp: time.Now()}}

		err = agg.UnmarshallPayloads(data2)
		require.NoError(t, err)

		// After merge: 3 names (totoro, porco rosso, ponyo)
		assert.ElementsMatch(t, []string{"totoro", "porco rosso", "ponyo"}, agg.GetNames())

		// totoro should have 2 payloads (one from each batch)
		totoroPayloads := agg.GetPayloadsByName("totoro")
		assert.Len(t, totoroPayloads, 2)

		// porco rosso should still have 1 payload (from the first batch only)
		assert.Len(t, agg.GetPayloadsByName("porco rosso"), 1)

		// ponyo should have 1 payload (from the second batch)
		assert.Len(t, agg.GetPayloadsByName("ponyo"), 1)
	})

	t.Run("Reset clears merged state", func(t *testing.T) {
		data, err := generateTestData()
		require.NoError(t, err)

		agg := newAggregator(parseMockPayloadItem)
		err = agg.UnmarshallPayloads(data)
		require.NoError(t, err)
		assert.NotEmpty(t, agg.GetNames())

		agg.Reset()
		assert.Empty(t, agg.GetNames())

		// After reset, new payloads start fresh (not merged with old)
		err = agg.UnmarshallPayloads(data)
		require.NoError(t, err)
		assert.Len(t, agg.GetPayloadsByName("totoro"), 1)
	})

	t.Run("UnmarshallPayloads does not print debug output to stdout", func(t *testing.T) {
		data, err := generateTestData()
		require.NoError(t, err)

		agg := newAggregator(parseMockPayloadItem)

		// Capture stdout during UnmarshallPayloads
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		err = agg.UnmarshallPayloads(data)
		w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()

		// The old code had a bare fmt.Print(reflect.TypeOf(agg).Name()) that printed
		// the aggregator type name to stdout on every call. Verify it's gone.
		assert.Empty(t, output, "UnmarshallPayloads should not print anything to stdout")
	})
}
