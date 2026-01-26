// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// The sketch's relative accuracy and maximum number of bins is identical
// to the one used in the trace-agent for consistency:
// https://github.com/DataDog/datadog-agent/blob/cbac965/pkg/trace/stats/statsraw.go#L18-L26
const (
	sketchRelativeAccuracy = 0.01
	sketchMaxBins          = 2048
)

func testSketchBytes(nums ...float64) []byte {
	sketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(sketchRelativeAccuracy, sketchMaxBins)
	if err != nil {
		// the only possible error is if the relative accuracy is < 0 or > 1;
		// we know that's not the case because it's a constant defined as 0.01
		panic(err)
	}
	for _, num := range nums {
		sketch.Add(num)
	}
	buf, err := proto.Marshal(sketch.ToProto())
	if err != nil {
		// there should be no error under any circumstances here
		panic(err)
	}
	return buf
}

func TestConversion(t *testing.T) {
	want := &pb.StatsPayload{
		Stats: []*pb.ClientStatsPayload{
			{
				Hostname:         "host",
				Env:              "prod",
				Version:          "v1.2",
				Lang:             "go",
				TracerVersion:    "v44",
				RuntimeID:        "123jkl",
				Sequence:         2,
				AgentAggregation: "blah",
				Service:          "mysql",
				ContainerID:      "abcdef123456",
				Tags:             []string{"a:b", "c:d"},
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    10,
						Duration: 1,
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "mysql",
								Name:           "db.query",
								Resource:       "UPDATE name",
								HTTPStatusCode: 100,
								Type:           "sql",
								DBType:         "postgresql",
								Synthetics:     true,
								Hits:           5,
								Errors:         2,
								Duration:       100,
								OkSummary:      testSketchBytes(1, 2, 3),
								ErrorSummary:   testSketchBytes(4, 5, 6),
								TopLevelHits:   3,
							},
							{
								Service:        "kafka",
								Name:           "queue.add",
								Resource:       "append",
								HTTPStatusCode: 220,
								Type:           "queue",
								Hits:           15,
								Errors:         3,
								Duration:       143,
								OkSummary:      nil,
								ErrorSummary:   nil,
								TopLevelHits:   5,
							},
						},
					},
					{
						Start:    20,
						Duration: 3,
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "php-go",
								Name:           "http.post",
								Resource:       "user_profile",
								HTTPStatusCode: 440,
								Type:           "web",
								Hits:           11,
								Errors:         3,
								Duration:       987,
								OkSummary:      testSketchBytes(7, 8),
								ErrorSummary:   testSketchBytes(9, 10, 11),
								TopLevelHits:   1,
							},
						},
					},
				},
			},
			{
				Hostname:         "host2",
				Env:              "staging",
				Version:          "v1.3",
				Lang:             "java",
				TracerVersion:    "v12",
				RuntimeID:        "12#12@",
				Sequence:         2,
				AgentAggregation: "blur",
				Service:          "sprint",
				ContainerID:      "kljdsfalk32",
				Tags:             []string{"x:y", "z:w"},
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    30,
						Duration: 5,
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "spring-web",
								Name:           "http.get",
								Resource:       "login",
								HTTPStatusCode: 200,
								Type:           "web",
								Hits:           12,
								Errors:         2,
								Duration:       13,
								OkSummary:      testSketchBytes(9, 7, 5),
								ErrorSummary:   testSketchBytes(9, 5, 2),
								TopLevelHits:   9,
							},
						},
					},
				},
			},
		},
	}

	t.Run("same", func(t *testing.T) {
		trans := &Translator{logger: zap.NewNop()}
		mx, err := trans.StatsToMetrics(want)
		assert.NoError(t, err)
		var results []*pb.StatsPayload
		for i := 0; i < mx.ResourceMetrics().Len(); i++ {
			rm := mx.ResourceMetrics().At(i)
			for j := 0; j < rm.ScopeMetrics().Len(); j++ {
				sm := rm.ScopeMetrics().At(j)
				for k := 0; k < sm.Metrics().Len(); k++ {
					md := sm.Metrics().At(k)
					// these metrics are an APM Stats payload; consume it as such
					for l := 0; l < md.Sum().DataPoints().Len(); l++ {
						if payload, ok := md.Sum().DataPoints().At(l).Attributes().Get(keyStatsPayload); ok {

							stats := &pb.StatsPayload{}
							err = proto.Unmarshal(payload.Bytes().AsRaw(), stats)
							assert.NoError(t, err)
							results = append(results, stats)
						}
					}
					assert.NoError(t, err)
				}
			}
		}

		assert.Len(t, results, 1)
		assert.True(t, proto.Equal(want, results[0]))
	})
}
