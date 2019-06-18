package writer

import (
	"math"
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestStatsWriter(t *testing.T) {
	// TODO
}

func TestStatsWriterBuildPayloads(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		assert := assert.New(t)

		sw := testStatsWriter()

		// This gives us a total of 45 entries. 3 per span, 5
		// spans per stat bucket. Each buckets have the same
		// time window (start: 0, duration 1e9).
		stats := []stats.Bucket{
			testutil.RandomBucket(5),
			testutil.RandomBucket(5),
			testutil.RandomBucket(5),
		}

		// Remove duplicates so that we have a predictable state. In another
		// case we'll test with duplicates.
		expectedNbEntries := removeDuplicateEntries(stats)

		expectedNbPayloads := int(math.Ceil(float64(expectedNbEntries) / 12))

		// Compute our expected number of entries by payload
		expectedNbEntriesByPayload := make([]int, expectedNbPayloads)
		for i := 0; i < expectedNbEntries; i++ {
			expectedNbEntriesByPayload[i%expectedNbPayloads]++
		}

		expectedCounts := countsByEntries(stats)

		payloads, nbStatBuckets, nbEntries := sw.buildPayloads(stats, 12)

		assert.Equal(expectedNbPayloads, len(payloads))
		assert.Equal(expectedNbPayloads, nbStatBuckets)
		assert.Equal(expectedNbEntries, nbEntries)

		for i := 0; i < expectedNbPayloads; i++ {
			assert.Equal(1, len(payloads[i].Stats))
			assert.Equal(expectedNbEntriesByPayload[i], len(payloads[i].Stats[0].Counts))
		}

		assertCountByEntries(assert, expectedCounts, payloads)
	})

	t.Run("dupes", func(t *testing.T) {
		rand.Seed(55)
		assert := assert.New(t)

		sw := testStatsWriter()

		// This gives us a total of 45 entries. 3 per span, 5
		// spans per stat bucket. Each buckets have the same
		// time window (start: 0, duration 1e9).
		stats := []stats.Bucket{
			testutil.RandomBucket(5),
			testutil.RandomBucket(5),
			testutil.RandomBucket(5),
		}

		// Remove duplicates so that we have a predictable
		// state.
		expectedNbEntries := removeDuplicateEntries(stats)

		// Ensure we have 45 - 2 entries, as we'll duplicate 2
		// of them.
		for ekey := range stats[0].Counts {
			if expectedNbEntries == 43 {
				break
			}

			delete(stats[0].Counts, ekey)
			expectedNbEntries--
		}

		// Force 2 duplicates
		i := 0
		for ekey, e := range stats[0].Counts {
			if i >= 2 {
				break
			}
			stats[1].Counts[ekey] = e
			i++
		}

		expectedNbPayloads := int(math.Ceil(float64(expectedNbEntries) / 12))

		// Compute our expected number of entries by payload
		expectedNbEntriesByPayload := make([]int, expectedNbPayloads)
		for i := 0; i < expectedNbEntries; i++ {
			expectedNbEntriesByPayload[i%expectedNbPayloads]++
		}

		expectedCounts := countsByEntries(stats)

		payloads, nbStatBuckets, nbEntries := sw.buildPayloads(stats, 12)

		assert.Equal(expectedNbPayloads, len(payloads))
		assert.Equal(expectedNbPayloads, nbStatBuckets)
		assert.Equal(expectedNbEntries, nbEntries)

		for i := 0; i < expectedNbPayloads; i++ {
			assert.Equal(1, len(payloads[i].Stats))
			assert.Equal(expectedNbEntriesByPayload[i], len(payloads[i].Stats[0].Counts))
		}

		assertCountByEntries(assert, expectedCounts, payloads)
	})

	t.Run("no-split", func(t *testing.T) {
		rand.Seed(1)
		assert := assert.New(t)

		sw := testStatsWriter()
		go sw.Run()
		defer sw.Stop()

		// This gives us a tota of 45 entries. 3 per span, 5 spans per
		// stat bucket. Each buckets have the same time window (start:
		// 0, duration 1e9).
		stats := []stats.Bucket{
			testutil.RandomBucket(5),
			testutil.RandomBucket(5),
			testutil.RandomBucket(5),
		}

		payloads, nbStatBuckets, nbEntries := sw.buildPayloads(stats, 1337)

		assert.Equal(1, len(payloads))
		assert.Equal(3, nbStatBuckets)
		assert.Equal(45, nbEntries)

		assert.Equal(3, len(payloads[0].Stats))
		assert.Equal(15, len(payloads[0].Stats[0].Counts))
		assert.Equal(15, len(payloads[0].Stats[1].Counts))
		assert.Equal(15, len(payloads[0].Stats[2].Counts))
	})
}

func testStatsWriter() *StatsWriter {
	in := make(chan []stats.Bucket)
	cfg := &config.AgentConfig{
		Hostname:   "testhost",
		DefaultEnv: "testing",
		Endpoints:  []*config.Endpoint{{Host: "http://test", APIKey: "123"}},
	}
	return NewStatsWriter(cfg, in)
}

func removeDuplicateEntries(stats []stats.Bucket) int {
	nbEntries := 0
	entries := make(map[string]struct{}, 45)
	for _, s := range stats {
		for ekey := range s.Counts {
			if _, ok := entries[ekey]; !ok {
				entries[ekey] = struct{}{}
				nbEntries++
			} else {
				delete(s.Counts, ekey)
			}
		}
	}
	return nbEntries
}

func countsByEntries(stats []stats.Bucket) map[string]float64 {
	counts := make(map[string]float64)
	for _, s := range stats {
		for k, c := range s.Counts {
			v, ok := counts[k]
			if !ok {
				v = 0
			}
			v += c.Value
			counts[k] = v
		}
	}
	return counts
}

func assertCountByEntries(assert *assert.Assertions, expectedCounts map[string]float64, payloads []*stats.Payload) {
	actualCounts := make(map[string]float64)
	for _, p := range payloads {
		for _, s := range p.Stats {
			for ekey, e := range s.Counts {
				v, ok := actualCounts[ekey]
				if !ok {
					v = 0
				}
				v += e.Value
				actualCounts[ekey] = v
			}
		}
	}

	assert.Equal(expectedCounts, actualCounts)
}
