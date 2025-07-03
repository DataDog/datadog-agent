// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-go/v5/statsd"
	"google.golang.org/protobuf/proto"
)

var fuzzer = fuzz.NewWithSeed(1)

func newTestAggregator() *ClientStatsAggregator {
	conf := &config.AgentConfig{
		DefaultEnv: "agentEnv",
		Hostname:   "agentHostname",
	}
	a := NewClientStatsAggregator(conf, noopStatsWriter{}, &statsd.NoOpClient{})
	a.Start()
	a.flushTicker.Stop()
	return a
}

type noopStatsWriter struct{}

func (noopStatsWriter) Write(*pb.StatsPayload) {}

type mockStatsWriter struct {
	payloads []*pb.StatsPayload
	mu       sync.Mutex
}

func (w *mockStatsWriter) Write(p *pb.StatsPayload) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payloads = append(w.payloads, p)
}

func (w *mockStatsWriter) Reset() []*pb.StatsPayload {
	w.mu.Lock()
	defer w.mu.Unlock()
	ret := w.payloads
	w.payloads = nil
	return ret
}

func payloadWithCounts(ts time.Time, k BucketsAggregationKey, containerID, version, imageTag, gitCommitSha, lang string, hits, errors, duration uint64) *pb.ClientStatsPayload {
	return &pb.ClientStatsPayload{
		Env:          "test-env",
		Version:      version,
		ImageTag:     imageTag,
		GitCommitSha: gitCommitSha,
		ContainerID:  containerID,
		Lang:         lang,
		Stats: []*pb.ClientStatsBucket{
			{
				Start: uint64(ts.UnixNano()),
				Stats: []*pb.ClientGroupedStats{
					{
						Service:        k.Service,
						Name:           k.Name,
						SpanKind:       k.SpanKind,
						Resource:       k.Resource,
						HTTPStatusCode: k.StatusCode,
						Type:           k.Type,
						Synthetics:     k.Synthetics,
						Hits:           hits,
						Errors:         errors,
						Duration:       duration,
						GRPCStatusCode: k.GRPCStatusCode,
					},
				},
			},
		},
	}
}

func getTestStatsWithStart(t *testing.T, start time.Time, incPeerTags bool) *pb.ClientStatsPayload {
	b := &pb.ClientStatsBucket{}
	fuzzer.Fuzz(b)

	b.Start = uint64(alignAggTs(start).UnixNano())
	b.Duration = uint64(clientBucketDuration.Nanoseconds())
	b.AgentTimeShift = 0

	stats := make([]*pb.ClientGroupedStats, 0, len(b.Stats))
	for _, s := range b.Stats {
		if s == nil {
			continue
		}
		if !incPeerTags {
			s.PeerTags = nil
		}
		s.DBType = ""
		s.OkSummary = encodeTestSketch(t, generateTestSketch(t))
		s.ErrorSummary = encodeTestSketch(t, generateTestSketch(t))
		stats = append(stats, s)
	}
	b.Stats = stats

	p := &pb.ClientStatsPayload{}
	fuzzer.Fuzz(p)
	p.Tags = nil
	p.TracerVersion = ""
	p.RuntimeID = ""
	p.ContainerID = ""
	p.Sequence = 0
	p.Service = ""
	p.Stats = []*pb.ClientStatsBucket{b}
	return p
}

func generateTestSketch(t *testing.T) *ddsketch.DDSketch {
	sketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	assert.NoError(t, err)
	for i := 0; i < 5; i++ {
		v := rand.NormFloat64()
		sketch.Add(v)
	}
	return normalizeSketch(sketch)
}

func encodeTestSketch(t *testing.T, s *ddsketch.DDSketch) []byte {
	msg := s.ToProto()
	data, err := proto.Marshal(msg)
	assert.NoError(t, err)
	return data
}

func assertAggCountsPayload(t *testing.T, aggCounts *pb.StatsPayload) {
	for _, p := range aggCounts.Stats {
		assert.Empty(t, p.TracerVersion)
		assert.Empty(t, p.RuntimeID)
		assert.Equal(t, uint64(0), p.Sequence)
		for _, s := range p.Stats {
			for _, b := range s.Stats {
				assert.Nil(t, b.OkSummary)
				assert.Nil(t, b.ErrorSummary)
			}
		}
	}
}

func assertAggStatsPayload(t *testing.T, aggStats *pb.StatsPayload) {
	for _, p := range aggStats.Stats {
		assert.Empty(t, p.TracerVersion)
		assert.Empty(t, p.RuntimeID)
		assert.Equal(t, uint64(0), p.Sequence)
		for _, s := range p.Stats {
			for _, b := range s.Stats {
				assert.NotNil(t, b.OkSummary)
				assert.NotNil(t, b.ErrorSummary)
			}
		}
	}
}

func duplicateStats(insertionTime time.Time, p *pb.ClientStatsPayload, times uint64) *pb.ClientStatsPayload {
	for _, s := range p.Stats {
		s.Start = uint64(alignAggTs(insertionTime).UnixNano())
		s.Duration = uint64(clientBucketDuration.Nanoseconds())
		s.AgentTimeShift = 0
		for _, stat := range s.Stats {
			if stat == nil {
				continue
			}
			stat.DBType = ""
			stat.Hits *= times
			stat.Errors *= times
			stat.Duration *= times
			stat.TopLevelHits *= times

			if stat.OkSummary != nil {
				okSumBytes := make([]byte, len(stat.OkSummary))
				copy(okSumBytes, stat.OkSummary)

				okSummary, _ := decodeSketch(stat.OkSummary)
				okSummary = normalizeSketch(okSummary)

				mergeSketch(okSummary, okSumBytes)
				stat.OkSummary, _ = proto.Marshal(okSummary.ToProto())
			}
			if stat.ErrorSummary != nil {
				errSumBytes := make([]byte, len(stat.ErrorSummary))
				copy(errSumBytes, stat.ErrorSummary)

				errSummary, _ := decodeSketch(stat.ErrorSummary)
				errSummary = normalizeSketch(errSummary)

				mergeSketch(errSummary, errSumBytes)

				stat.ErrorSummary, _ = proto.Marshal(errSummary.ToProto())
			}

		}
	}
	return p
}

func asserEqualPayloads(t *testing.T, p1, p2 *pb.ClientStatsPayload) {
	assert.Equal(t, p1.Service, p2.Service)
	assert.Equal(t, p1.ContainerID, p2.ContainerID)
	for i, csb := range p1.Stats {
		assert.Equal(t, csb.Start, p2.Stats[i].Start)
		assert.Equal(t, csb.Duration, p2.Stats[i].Duration)
		assert.Equal(t, csb.AgentTimeShift, p2.Stats[i].AgentTimeShift)
		assert.Equal(t, len(csb.Stats), len(p2.Stats[i].Stats))
		assert.ElementsMatch(t, csb.Stats, p2.Stats[i].Stats)
	}
}

func TestAggregatorFlushTime(t *testing.T) {
	assert := assert.New(t)
	a := newTestAggregator()
	msw := &mockStatsWriter{}
	a.writer = msw
	testTime := time.Now()
	a.flushOnTime(testTime)
	assert.Len(msw.payloads, 0)
	testPayload := getTestStatsWithStart(t, testTime, false)
	a.add(testTime, deepCopy(testPayload))
	a.flushOnTime(testTime)
	assert.Len(msw.payloads, 0)
	a.flushOnTime(testTime.Add(oldestBucketStart - bucketDuration))
	assert.Len(msw.payloads, 0)
	a.flushOnTime(testTime.Add(oldestBucketStart))
	require.NotEmpty(t, msw.payloads)
	s := msw.payloads[0]

	asserEqualPayloads(t, testPayload, s.Stats[0])
	assert.Len(a.buckets, 0)
}

func TestMergeMany(t *testing.T) {
	assert := assert.New(t)
	for i := 0; i < 10; i++ {
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		payloadTime := time.Now().Truncate(bucketDuration)
		merge1 := getTestStatsWithStart(t, payloadTime, false)
		merge2 := getTestStatsWithStart(t, payloadTime.Add(time.Nanosecond), false)
		other := getTestStatsWithStart(t, payloadTime.Add(-time.Nanosecond), false)
		merge3 := getTestStatsWithStart(t, payloadTime.Add(time.Second-time.Nanosecond), false)

		insertionTime := payloadTime.Add(time.Second)
		a.add(insertionTime, deepCopy(merge1))
		a.add(insertionTime, deepCopy(merge2))
		a.add(insertionTime, deepCopy(other))
		a.add(insertionTime, deepCopy(merge3))
		assert.Len(msw.payloads, 0) // no flush based on item count
		a.flushOnTime(payloadTime.Add(oldestBucketStart - time.Nanosecond))
		assert.Len(msw.payloads, 1)
		asserEqualPayloads(t, other, msw.payloads[0].Stats[0])

		a.flushOnTime(payloadTime.Add(oldestBucketStart))
		assert.Len(msw.payloads, 2)
		assertAggStatsPayload(t, msw.payloads[1])
		assert.Len(a.buckets, 0)
	}
}

func TestConcentratorAggregatorNotAligned(t *testing.T) {
	var ts time.Time
	bsize := clientBucketDuration.Nanoseconds()
	for i := 0; i < 50; i++ {
		fuzzer.Fuzz(&ts)
		aggTs := alignAggTs(ts)
		assert.True(t, aggTs.UnixNano()%bsize != 0)
		concentratorTs := alignTs(ts.UnixNano(), bsize)
		assert.True(t, concentratorTs%bsize == 0)
	}
}

func TestTimeShifts(t *testing.T) {
	type tt struct {
		shift, expectedShift time.Duration
		name                 string
	}
	tts := []tt{
		{
			shift:         100 * time.Hour,
			expectedShift: 100 * time.Hour,
			name:          "future",
		},
		{
			shift:         -11 * time.Hour,
			expectedShift: -11*time.Hour + oldestBucketStart - bucketDuration,
			name:          "past",
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			a := newTestAggregator()
			msw := &mockStatsWriter{}
			a.writer = msw
			agentTime := alignAggTs(time.Now())
			payloadTime := agentTime.Add(tc.shift)

			stats := getTestStatsWithStart(t, payloadTime, false)
			a.add(agentTime, deepCopy(stats))
			a.flushOnTime(agentTime)
			assert.Len(msw.payloads, 0)
			a.flushOnTime(agentTime.Add(oldestBucketStart + time.Nanosecond))
			require.Len(t, msw.payloads, 1)

			//stats.Stats[0].AgentTimeShift = -tc.expectedShift.Nanoseconds()
			stats.Stats[0].Start -= uint64(tc.expectedShift.Nanoseconds())

			s := msw.payloads[0]
			asserEqualPayloads(t, stats, s.Stats[0])
		})
	}
}

func TestFuzzCountFields(t *testing.T) {
	assert := assert.New(t)
	//for i := 0; i < 30; i++ {
	for i := 0; i < 1; i++ {
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		// Ensure that peer tags aggregation is on. Some tests may expect non-empty values the peer tags.
		payloadTime := time.Now().Truncate(bucketDuration)
		merge1 := getTestStatsWithStart(t, payloadTime, false)

		insertionTime := payloadTime.Add(time.Second)
		a.add(insertionTime, deepCopy(merge1))
		a.add(insertionTime, deepCopy(merge1))
		a.flushOnTime(payloadTime.Add(oldestBucketStart))
		require.Len(t, msw.payloads, 1)

		s := msw.payloads[0]
		asserEqualPayloads(t, duplicateStats(insertionTime, merge1, 2), s.Stats[0])

		assert.Len(a.buckets, 0)
	}
}

func TestCountAggregation(t *testing.T) {
	assert := assert.New(t)
	type tt struct {
		k    BucketsAggregationKey
		res  *pb.ClientGroupedStats
		name string
	}
	tts := []tt{
		{
			BucketsAggregationKey{Service: "s"},
			&pb.ClientGroupedStats{Service: "s"},
			"service",
		},
		{
			BucketsAggregationKey{Name: "n"},
			&pb.ClientGroupedStats{Name: "n"},
			"name",
		},
		{
			BucketsAggregationKey{Resource: "r"},
			&pb.ClientGroupedStats{Resource: "r"},
			"resource",
		},
		{
			BucketsAggregationKey{Type: "t"},
			&pb.ClientGroupedStats{Type: "t"},
			"resource",
		},
		{
			BucketsAggregationKey{Synthetics: true},
			&pb.ClientGroupedStats{Synthetics: true},
			"synthetics",
		},
		{
			BucketsAggregationKey{StatusCode: 10},
			&pb.ClientGroupedStats{HTTPStatusCode: 10},
			"status",
		},
		{
			BucketsAggregationKey{GRPCStatusCode: "2"},
			&pb.ClientGroupedStats{GRPCStatusCode: "2"},
			"status",
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAggregator()
			msw := &mockStatsWriter{}
			a.writer = msw
			testTime := time.Unix(time.Now().Unix(), 0)

			c1 := payloadWithCounts(testTime, tc.k, "", "test-version", "", "", "", 11, 7, 100)
			c2 := payloadWithCounts(testTime, tc.k, "", "test-version", "", "", "", 27, 2, 300)
			c3 := payloadWithCounts(testTime, tc.k, "", "test-version", "", "", "", 5, 10, 3)
			keyDefault := BucketsAggregationKey{}
			cDefault := payloadWithCounts(testTime, keyDefault, "", "test-version", "", "", "", 0, 2, 4)

			assert.Len(msw.payloads, 0)
			a.add(testTime, deepCopy(c1))
			a.add(testTime, deepCopy(c2))
			a.add(testTime, deepCopy(c3))
			a.add(testTime, deepCopy(cDefault))
			assert.Len(msw.payloads, 0)
			a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
			require.Len(t, msw.payloads, 1)

			payload := msw.payloads[0]
			assertAggCountsPayload(t, payload)

			tc.res.Hits = 43
			tc.res.Errors = 19
			tc.res.Duration = 403
			assert.ElementsMatch(payload.Stats[0].Stats[0].Stats, []*pb.ClientGroupedStats{
				tc.res,
				// Additional grouped stat object that corresponds to the keyDefault/cDefault.
				// We do not expect this to be aggregated with the non-default key in the test.
				{
					Hits:     0,
					Errors:   2,
					Duration: 4,
				},
			})
			assert.Len(a.buckets, 0)
		})
	}
}

func TestCountAggregationPeerTags(t *testing.T) {
	type tt struct {
		k        BucketsAggregationKey
		res      *pb.ClientGroupedStats
		name     string
		peerTags []string
	}
	// The fnv64a hash of the peerTags var.
	peerTagsHash := uint64(8580633704111928789)
	peerTags := []string{"db.instance:a", "db.system:b", "peer.service:remote-service"}
	tts := []tt{
		{
			BucketsAggregationKey{Service: "s", Name: "test.op"},
			&pb.ClientGroupedStats{Service: "s", Name: "test.op"},
			"peer tags aggregation disabled",
			nil,
		},
		{
			BucketsAggregationKey{Service: "s", PeerTagsHash: peerTagsHash},
			&pb.ClientGroupedStats{Service: "s", PeerTags: peerTags},
			"peer tags aggregation enabled",
			peerTags,
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			a := newTestAggregator()
			msw := &mockStatsWriter{}
			a.writer = msw
			testTime := time.Unix(time.Now().Unix(), 0)

			c1 := payloadWithCounts(testTime, tc.k, "", "test-version", "", "", "", 11, 7, 100)
			c2 := payloadWithCounts(testTime, tc.k, "", "test-version", "", "", "", 27, 2, 300)
			c3 := payloadWithCounts(testTime, tc.k, "", "test-version", "", "", "", 5, 10, 3)
			c1.Stats[0].Stats[0].PeerTags = tc.peerTags
			c2.Stats[0].Stats[0].PeerTags = tc.peerTags
			c3.Stats[0].Stats[0].PeerTags = tc.peerTags
			keyDefault := BucketsAggregationKey{}
			cDefault := payloadWithCounts(testTime, keyDefault, "", "test-version", "", "", "", 0, 2, 4)

			assert.Len(msw.payloads, 0)
			a.add(testTime, deepCopy(c1))
			a.add(testTime, deepCopy(c2))
			a.add(testTime, deepCopy(c3))
			a.add(testTime, deepCopy(cDefault))
			assert.Len(msw.payloads, 0)
			a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
			require.Len(t, msw.payloads, 1)

			aggCounts := msw.payloads[0]
			assertAggCountsPayload(t, aggCounts)

			tc.res.Hits = 43
			tc.res.Errors = 19
			tc.res.Duration = 403
			assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []*pb.ClientGroupedStats{
				tc.res,
				// Additional grouped stat object that corresponds to the keyDefault/cDefault.
				// We do not expect this to be aggregated with the non-default key in the test.
				{
					Hits:     0,
					Errors:   2,
					Duration: 4,
				},
			})
			assert.Len(a.buckets, 0)
		})
	}
}

func TestAggregationVersionData(t *testing.T) {
	// Version data refers to all of: Version, GitCommitSha, and ImageTag.
	t.Run("all version data provided in payload", func(t *testing.T) {
		assert := assert.New(t)
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		testTime := time.Unix(time.Now().Unix(), 0)

		bak := BucketsAggregationKey{Service: "s", Name: "test.op"}
		c1 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 11, 7, 100)
		c2 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 27, 2, 300)
		c3 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 5, 10, 3)
		keyDefault := BucketsAggregationKey{}
		cDefault := payloadWithCounts(testTime, keyDefault, "1", "test-version", "abc", "abc123", "", 0, 2, 4)

		assert.Len(msw.payloads, 0)
		a.add(testTime, deepCopy(c1))
		a.add(testTime, deepCopy(c2))
		a.add(testTime, deepCopy(c3))
		a.add(testTime, deepCopy(cDefault))
		assert.Len(msw.payloads, 0)
		a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
		require.Len(t, msw.payloads, 1)

		aggCounts := msw.payloads[0]
		assertAggCountsPayload(t, aggCounts)

		expectedRes := &pb.ClientGroupedStats{
			Service:  bak.Service,
			Name:     bak.Name,
			Hits:     43,
			Errors:   19,
			Duration: 403,
		}
		assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []*pb.ClientGroupedStats{
			expectedRes,
			// Additional grouped stat object that corresponds to the keyDefault/cDefault.
			// We do not expect this to be aggregated with the non-default key in the test.
			{
				Hits:     0,
				Errors:   2,
				Duration: 4,
			},
		})
		assert.Equal("test-version", aggCounts.Stats[0].Version)
		assert.Equal("abc", aggCounts.Stats[0].ImageTag)
		assert.Equal("abc123", aggCounts.Stats[0].GitCommitSha)
		assert.Len(a.buckets, 0)
	})

	t.Run("git commit sha and image tag come from container tags", func(t *testing.T) {
		assert := assert.New(t)
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		cfg := config.New()
		cfg.ContainerTags = func(_ string) ([]string, error) {
			return []string{"git.commit.sha:sha-from-container-tags", "image_tag:image-tag-from-container-tags"}, nil
		}
		a.conf = cfg
		testTime := time.Unix(time.Now().Unix(), 0)

		bak := BucketsAggregationKey{Service: "s", Name: "test.op"}
		c1 := payloadWithCounts(testTime, bak, "1", "", "", "", "", 11, 7, 100)
		c2 := payloadWithCounts(testTime, bak, "1", "", "", "", "", 27, 2, 300)
		c3 := payloadWithCounts(testTime, bak, "1", "", "", "", "", 5, 10, 3)
		keyDefault := BucketsAggregationKey{}
		cDefault := payloadWithCounts(testTime, keyDefault, "1", "", "", "", "", 0, 2, 4)

		assert.Len(msw.payloads, 0)
		a.add(testTime, deepCopy(c1))
		a.add(testTime, deepCopy(c2))
		a.add(testTime, deepCopy(c3))
		a.add(testTime, deepCopy(cDefault))
		assert.Len(msw.payloads, 0)
		a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
		require.Len(t, msw.payloads, 1)

		// Add the expected gitCommitSha and imageTag on c1, c2, c3, and cDefault for these assertions.
		c1.GitCommitSha = "sha-from-container-tags"
		c1.ImageTag = "image-tag-from-container-tags"
		c2.GitCommitSha = "sha-from-container-tags"
		c2.ImageTag = "image-tag-from-container-tags"
		c3.GitCommitSha = "sha-from-container-tags"
		c3.ImageTag = "image-tag-from-container-tags"
		cDefault.GitCommitSha = "sha-from-container-tags"
		cDefault.ImageTag = "image-tag-from-container-tags"

		aggCounts := msw.payloads[0]
		assertAggCountsPayload(t, aggCounts)

		expectedRes := &pb.ClientGroupedStats{
			Service:  bak.Service,
			Name:     bak.Name,
			Hits:     43,
			Errors:   19,
			Duration: 403,
		}
		assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []*pb.ClientGroupedStats{
			expectedRes,
			// Additional grouped stat object that corresponds to the keyDefault/cDefault.
			// We do not expect this to be aggregated with the non-default key in the test.
			{
				Hits:     0,
				Errors:   2,
				Duration: 4,
			},
		})
		assert.Equal("", aggCounts.Stats[0].Version)
		assert.Equal("image-tag-from-container-tags", aggCounts.Stats[0].ImageTag)
		assert.Equal("sha-from-container-tags", aggCounts.Stats[0].GitCommitSha)
		assert.Len(a.buckets, 0)
	})

	t.Run("payload git commit sha and image tag override container tags", func(t *testing.T) {
		assert := assert.New(t)
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		cfg := config.New()
		cfg.ContainerTags = func(_ string) ([]string, error) {
			return []string{"git.commit.sha:overrideThisSha", "image_tag:overrideThisImageTag"}, nil
		}
		a.conf = cfg
		testTime := time.Unix(time.Now().Unix(), 0)

		bak := BucketsAggregationKey{Service: "s", Name: "test.op"}
		c1 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 11, 7, 100)
		c2 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 27, 2, 300)
		c3 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 5, 10, 3)
		keyDefault := BucketsAggregationKey{}
		cDefault := payloadWithCounts(testTime, keyDefault, "1", "test-version", "abc", "abc123", "", 0, 2, 4)

		assert.Len(msw.payloads, 0)
		a.add(testTime, deepCopy(c1))
		a.add(testTime, deepCopy(c2))
		a.add(testTime, deepCopy(c3))
		a.add(testTime, deepCopy(cDefault))
		assert.Len(msw.payloads, 0)
		a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
		require.Len(t, msw.payloads, 1)

		aggCounts := msw.payloads[0]
		assertAggCountsPayload(t, aggCounts)

		expectedRes := &pb.ClientGroupedStats{
			Service:  bak.Service,
			Name:     bak.Name,
			Hits:     43,
			Errors:   19,
			Duration: 403,
		}
		assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []*pb.ClientGroupedStats{
			expectedRes,
			// Additional grouped stat object that corresponds to the keyDefault/cDefault.
			// We do not expect this to be aggregated with the non-default key in the test.
			{
				Hits:     0,
				Errors:   2,
				Duration: 4,
			},
		})
		assert.Equal("test-version", aggCounts.Stats[0].Version)
		assert.Equal("abc", aggCounts.Stats[0].ImageTag)
		assert.Equal("abc123", aggCounts.Stats[0].GitCommitSha)
		assert.Len(a.buckets, 0)
	})
}

func TestAggregationProcessTags(t *testing.T) {
	assert := assert.New(t)
	a := newTestAggregator()
	msw := &mockStatsWriter{}
	a.writer = msw
	testTime := time.Unix(time.Now().Unix(), 0)

	bak := BucketsAggregationKey{Service: "s", Name: "test.op"}
	c1 := payloadWithCounts(testTime, bak, "", "test-version", "abc", "abc123", "", 11, 7, 100)
	c1.ProcessTags = "a:1,b:2,c:3"
	c2 := payloadWithCounts(testTime, bak, "", "test-version", "abc", "abc123", "", 11, 7, 100)
	c2.ProcessTags = "b:33"

	assert.Len(msw.payloads, 0)
	a.add(testTime, deepCopy(c1))
	a.add(testTime, deepCopy(c2))
	assert.Len(msw.payloads, 0)
	a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
	require.Len(t, msw.payloads, 1)

	aggCounts := msw.payloads[0]
	assertAggCountsPayload(t, aggCounts)

	assert.Len(aggCounts.Stats, 2)
	resProcessTags := []string{aggCounts.Stats[0].ProcessTags, aggCounts.Stats[1].ProcessTags}
	resProcessTagsHash := []uint64{aggCounts.Stats[0].ProcessTagsHash, aggCounts.Stats[1].ProcessTagsHash}
	assert.ElementsMatch([]string{"a:1,b:2,c:3", "b:33"}, resProcessTags)
	assert.ElementsMatch([]uint64{7030721150995765661, 6360281807028847755}, resProcessTagsHash)
	assert.Len(a.buckets, 0)
}

func TestAggregationContainerID(t *testing.T) {
	t.Run("ContainerID empty", func(t *testing.T) {
		assert := assert.New(t)
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		testTime := time.Unix(time.Now().Unix(), 0)

		bak := BucketsAggregationKey{Service: "s", Name: "test.op"}
		c1 := payloadWithCounts(testTime, bak, "", "test-version", "abc", "abc123", "", 11, 7, 100)
		c2 := payloadWithCounts(testTime, bak, "", "test-version", "abc", "abc123", "", 27, 2, 300)
		c3 := payloadWithCounts(testTime, bak, "", "test-version", "abc", "abc123", "", 5, 10, 3)
		keyDefault := BucketsAggregationKey{}
		cDefault := payloadWithCounts(testTime, keyDefault, "", "test-version", "abc", "abc123", "", 0, 2, 4)

		assert.Len(msw.payloads, 0)
		a.add(testTime, deepCopy(c1))
		a.add(testTime, deepCopy(c2))
		a.add(testTime, deepCopy(c3))
		a.add(testTime, deepCopy(cDefault))
		assert.Len(msw.payloads, 0)
		a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
		require.Len(t, msw.payloads, 1)

		aggCounts := msw.payloads[0]
		assertAggCountsPayload(t, aggCounts)

		assert.Equal(aggCounts.Stats[0].ContainerID, "")
		assert.Len(a.buckets, 0)
	})

	t.Run("ContainerID not empty", func(t *testing.T) {
		assert := assert.New(t)
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		testTime := time.Unix(time.Now().Unix(), 0)

		bak := BucketsAggregationKey{Service: "s", Name: "test.op"}
		c1 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 11, 7, 100)
		c2 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 27, 2, 300)
		c3 := payloadWithCounts(testTime, bak, "1", "test-version", "abc", "abc123", "", 5, 10, 3)
		keyDefault := BucketsAggregationKey{}
		cDefault := payloadWithCounts(testTime, keyDefault, "1", "test-version", "abc", "abc123", "", 0, 2, 4)

		assert.Len(msw.payloads, 0)
		a.add(testTime, deepCopy(c1))
		a.add(testTime, deepCopy(c2))
		a.add(testTime, deepCopy(c3))
		a.add(testTime, deepCopy(cDefault))
		assert.Len(msw.payloads, 0)
		a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
		require.Len(t, msw.payloads, 1)

		aggCounts := msw.payloads[0]
		assertAggCountsPayload(t, aggCounts)

		assert.Equal(aggCounts.Stats[0].ContainerID, "1")
		assert.Len(a.buckets, 0)
	})
}

func TestLangAggregation(t *testing.T) {
	t.Run("different_lang_separate_buckets", func(t *testing.T) {
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		testTime := time.Unix(time.Now().Unix(), 0)

		bak := BucketsAggregationKey{Service: "test-service"}
		c1 := payloadWithCounts(testTime, bak, "", "1.0.0", "", "", "go", 10, 1, 100)
		c2 := payloadWithCounts(testTime, bak, "", "1.0.0", "", "", "python", 5, 2, 200)

		a.add(testTime, deepCopy(c1))
		a.add(testTime, deepCopy(c2))
		a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))

		require.Len(t, msw.payloads, 1)
		assert.Len(t, msw.payloads[0].Stats, 2) // Separate buckets for different languages
	})

	t.Run("same_lang_same_bucket", func(t *testing.T) {
		a := newTestAggregator()
		msw := &mockStatsWriter{}
		a.writer = msw
		testTime := time.Unix(time.Now().Unix(), 0)

		bak := BucketsAggregationKey{Service: "test-service"}
		c1 := payloadWithCounts(testTime, bak, "", "1.0.0", "", "", "go", 10, 1, 100)
		c2 := payloadWithCounts(testTime, bak, "", "1.0.0", "", "", "go", 5, 2, 200)

		a.add(testTime, deepCopy(c1))
		a.add(testTime, deepCopy(c2))
		a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))

		require.Len(t, msw.payloads, 1)
		require.Len(t, msw.payloads[0].Stats, 1) // Same bucket for same lang

		payload := msw.payloads[0].Stats[0]
		assert.Equal(t, "go", payload.Lang)
		assert.Equal(t, uint64(15), payload.Stats[0].Stats[0].Hits) // 10 + 5
	})
}

func TestNewBucketAggregationKeyPeerTags(t *testing.T) {
	// The hash of "peer.service:remote-service".
	peerTagsHash := uint64(3430395298086625290)
	t.Run("disabled", func(t *testing.T) {
		assert := assert.New(t)
		r := newBucketAggregationKey(&pb.ClientGroupedStats{Service: "a"})
		assert.Equal(BucketsAggregationKey{Service: "a"}, r)
	})
	t.Run("enabled", func(t *testing.T) {
		assert := assert.New(t)
		r := newBucketAggregationKey(&pb.ClientGroupedStats{Service: "a", PeerTags: []string{"peer.service:remote-service"}})
		assert.Equal(BucketsAggregationKey{Service: "a", PeerTagsHash: peerTagsHash}, r)
	})
}

func deepCopy(p *pb.ClientStatsPayload) *pb.ClientStatsPayload {
	payload := &pb.ClientStatsPayload{
		Hostname:         p.GetHostname(),
		Env:              p.GetEnv(),
		Version:          p.GetVersion(),
		Lang:             p.GetLang(),
		TracerVersion:    p.GetTracerVersion(),
		RuntimeID:        p.GetRuntimeID(),
		Sequence:         p.GetSequence(),
		AgentAggregation: p.GetAgentAggregation(),
		Service:          p.GetService(),
		ContainerID:      p.GetContainerID(),
		Tags:             p.GetTags(),
		GitCommitSha:     p.GetGitCommitSha(),
		ImageTag:         p.GetImageTag(),
		ProcessTags:      p.GetProcessTags(),
		ProcessTagsHash:  p.GetProcessTagsHash(),
	}
	payload.Stats = deepCopyStatsBucket(p.Stats)
	return payload
}

func deepCopyStatsBucket(s []*pb.ClientStatsBucket) []*pb.ClientStatsBucket {
	if s == nil {
		return nil
	}
	bucket := make([]*pb.ClientStatsBucket, len(s))
	for i, b := range s {
		bucket[i] = &pb.ClientStatsBucket{
			Start:          b.GetStart(),
			Duration:       b.GetDuration(),
			AgentTimeShift: b.GetAgentTimeShift(),
		}
		bucket[i].Stats = deepCopyGroupedStats(b.Stats)
	}
	return bucket
}

func deepCopyGroupedStats(s []*pb.ClientGroupedStats) []*pb.ClientGroupedStats {
	if s == nil {
		return nil
	}
	stats := make([]*pb.ClientGroupedStats, len(s))
	for i, b := range s {
		if b == nil {
			stats[i] = nil
			continue
		}

		stats[i] = &pb.ClientGroupedStats{
			Service:        b.GetService(),
			Name:           b.GetName(),
			Resource:       b.GetResource(),
			HTTPStatusCode: b.GetHTTPStatusCode(),
			Type:           b.GetType(),
			DBType:         b.GetDBType(),
			Hits:           b.GetHits(),
			Errors:         b.GetErrors(),
			Duration:       b.GetDuration(),
			Synthetics:     b.GetSynthetics(),
			TopLevelHits:   b.GetTopLevelHits(),
			SpanKind:       b.GetSpanKind(),
			PeerTags:       b.GetPeerTags(),
			IsTraceRoot:    b.GetIsTraceRoot(),
			GRPCStatusCode: b.GetGRPCStatusCode(),
		}
		if b.OkSummary != nil {
			stats[i].OkSummary = make([]byte, len(b.OkSummary))
			copy(stats[i].OkSummary, b.OkSummary)
		}
		if b.ErrorSummary != nil {
			stats[i].ErrorSummary = make([]byte, len(b.ErrorSummary))
			copy(stats[i].ErrorSummary, b.ErrorSummary)
		}
	}
	return stats
}
