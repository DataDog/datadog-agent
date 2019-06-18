package writer

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/stretchr/testify/assert"
)

func TestStatsWriter_StatHandling(t *testing.T) {
	assert := assert.New(t)

	// Given a stats writer, its incoming channel and the endpoint that receives the payloads
	statsWriter, statsChannel, testEndpoint, _ := testStatsWriter()

	statsWriter.Start()

	// Given 2 slices of 3 test buckets
	testStats1 := []stats.Bucket{
		testutil.RandomBucket(3),
		testutil.RandomBucket(3),
		testutil.RandomBucket(3),
	}
	testStats2 := []stats.Bucket{
		testutil.RandomBucket(3),
		testutil.RandomBucket(3),
		testutil.RandomBucket(3),
	}

	// When sending those slices
	statsChannel <- testStats1
	statsChannel <- testStats2

	// And stopping stats writer
	close(statsChannel)
	statsWriter.Stop()

	payloads := testEndpoint.SuccessPayloads()

	// Then the endpoint should have received 2 payloads, containing all stat buckets
	assert.Len(payloads, 2, "There should be 2 payloads")

	payload1 := payloads[0]
	payload2 := payloads[1]

	expectedHeaders := map[string]string{
		"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
		"Content-Type":                 "application/json",
		"Content-Encoding":             "gzip",
	}

	assertPayload(assert, expectedHeaders, testStats1, payload1)
	assertPayload(assert, expectedHeaders, testStats2, payload2)
}

func TestStatsWriter_UpdateInfoHandling(t *testing.T) {
	rand.Seed(1)
	assert := assert.New(t)

	// Given a stats writer, its incoming channel and the endpoint that receives the payloads
	statsWriter, statsChannel, testEndpoint, statsClient := testStatsWriter()
	statsWriter.conf.UpdateInfoPeriod = 100 * time.Millisecond

	statsWriter.Start()

	expectedNumPayloads := int64(0)
	expectedNumBuckets := int64(0)
	expectedNumBytes := int64(0)
	expectedMinNumRetries := int64(0)
	expectedNumErrors := int64(0)

	// When sending 1 payload with 3 buckets
	expectedNumPayloads++
	payload1Buckets := []stats.Bucket{
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
	}
	statsChannel <- payload1Buckets
	expectedNumBuckets += 3
	expectedNumBytes += calculateStatPayloadSize(payload1Buckets)

	// And another one with another 3 buckets
	expectedNumPayloads++
	payload2Buckets := []stats.Bucket{
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
	}
	statsChannel <- payload2Buckets
	expectedNumBuckets += 3
	expectedNumBytes += calculateStatPayloadSize(payload2Buckets)

	// Wait for previous payloads to be sent
	time.Sleep(2 * statsWriter.conf.UpdateInfoPeriod)

	// And then sending a third payload with other 3 buckets with an errored out endpoint
	testEndpoint.SetError(fmt.Errorf("non retriable error"))
	expectedNumErrors++
	payload3Buckets := []stats.Bucket{
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
	}
	statsChannel <- payload3Buckets
	expectedNumBuckets += 3
	expectedNumBytes += calculateStatPayloadSize(payload3Buckets)

	// And waiting for twice the flush period to trigger payload sending and info updating
	time.Sleep(2 * statsWriter.conf.UpdateInfoPeriod)

	// And then sending a third payload with other 3 traces with an errored out endpoint with retry
	testEndpoint.SetError(&retriableError{
		err:      fmt.Errorf("non retriable error"),
		endpoint: testEndpoint,
	})
	expectedMinNumRetries++
	payload4Buckets := []stats.Bucket{
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
		testutil.RandomBucket(5),
	}
	statsChannel <- payload4Buckets
	expectedNumBuckets += 3
	expectedNumBytes += calculateStatPayloadSize(payload4Buckets)

	// And waiting for twice the flush period to trigger payload sending and info updating
	time.Sleep(2 * statsWriter.conf.UpdateInfoPeriod)

	close(statsChannel)
	statsWriter.Stop()

	// Then we expect some counts to have been sent to the stats client for each update tick (there should have been
	// at least 3 ticks)
	countSummaries := statsClient.GetCountSummaries()

	// Payload counts
	payloadSummary := countSummaries["datadog.trace_agent.stats_writer.payloads"]
	assert.True(len(payloadSummary.Calls) >= 3, "There should have been multiple payload count calls")
	assert.Equal(expectedNumPayloads, payloadSummary.Sum)

	// Traces counts
	bucketsSummary := countSummaries["datadog.trace_agent.stats_writer.stats_buckets"]
	assert.True(len(bucketsSummary.Calls) >= 3, "There should have been multiple stats_buckets count calls")
	assert.Equal(expectedNumBuckets, bucketsSummary.Sum)

	// Bytes counts
	bytesSummary := countSummaries["datadog.trace_agent.stats_writer.bytes"]
	assert.True(len(bytesSummary.Calls) >= 3, "There should have been multiple bytes count calls")
	assert.Equal(expectedNumBytes, bytesSummary.Sum)

	// Retry counts
	retriesSummary := countSummaries["datadog.trace_agent.stats_writer.retries"]
	assert.True(len(retriesSummary.Calls) >= 2, "There should have been multiple retries count calls")
	assert.True(retriesSummary.Sum >= expectedMinNumRetries)

	// Error counts
	errorsSummary := countSummaries["datadog.trace_agent.stats_writer.errors"]
	assert.True(len(errorsSummary.Calls) >= 3, "There should have been multiple errors count calls")
	assert.Equal(expectedNumErrors, errorsSummary.Sum)
}

func TestStatsWriter_BuildPayloads(t *testing.T) {
	t.Run("common case, no duplicate entries", func(t *testing.T) {
		assert := assert.New(t)

		sw, _, _, _ := testStatsWriter()

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

	t.Run("common case, with duplicate entries", func(t *testing.T) {
		rand.Seed(55)
		assert := assert.New(t)

		sw, _, _, _ := testStatsWriter()

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

	t.Run("no need for split", func(t *testing.T) {
		rand.Seed(1)
		assert := assert.New(t)

		sw, _, _, _ := testStatsWriter()
		sw.Start()

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

func calculateStatPayloadSize(buckets []stats.Bucket) int64 {
	statsPayload := &stats.Payload{
		HostName: testHostName,
		Env:      testEnv,
		Stats:    buckets,
	}

	var buf bytes.Buffer
	stats.EncodePayload(&buf, statsPayload)
	return int64(buf.Len())
}

func assertPayload(assert *assert.Assertions, headers map[string]string, buckets []stats.Bucket, p *payload) {
	statsPayload := stats.Payload{}

	reader := bytes.NewBuffer(p.bytes)
	gzipReader, err := gzip.NewReader(reader)

	assert.NoError(err, "Gzip reader should work correctly")

	jsonDecoder := json.NewDecoder(gzipReader)

	assert.NoError(jsonDecoder.Decode(&statsPayload), "Stats payload should unmarshal correctly")

	assert.Equal(headers, p.headers, "Headers should match expectation")
	assert.Equal(testHostName, statsPayload.HostName, "Hostname should match expectation")
	assert.Equal(testEnv, statsPayload.Env, "Env should match expectation")
	assert.Equal(buckets, statsPayload.Stats, "Stat buckets should match expectation")
}

func testStatsWriter() (*StatsWriter, chan []stats.Bucket, *testEndpoint, *testutil.TestStatsClient) {
	statsChannel := make(chan []stats.Bucket)
	conf := &config.AgentConfig{
		Hostname:          testHostName,
		DefaultEnv:        testEnv,
		StatsWriterConfig: writerconfig.DefaultStatsWriterConfig(),
	}
	statsWriter := NewStatsWriter(conf, statsChannel)
	testEndpoint := &testEndpoint{}
	statsWriter.sender.setEndpoint(testEndpoint)
	testStatsClient := metrics.Client.(*testutil.TestStatsClient)
	testStatsClient.Reset()

	return statsWriter, statsChannel, testEndpoint, testStatsClient
}
