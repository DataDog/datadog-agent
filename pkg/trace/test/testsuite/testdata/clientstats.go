package testdata

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/golang/protobuf/proto"
)

func getEmptyDDSketch() []byte {
	m, _ := mapping.NewLogarithmicMapping(0.01)
	s := ddsketch.NewDDSketch(m, store.NewDenseStore(), store.NewDenseStore())
	data, _ := proto.Marshal(s.ToProto())
	return data
}

// ClientStatsTests contains a suite of tests for testing the stats endpoint.
var ClientStatsTests = []struct {
	In  pb.ClientStatsPayload
	Out pb.StatsPayload
}{
	{
		In: pb.ClientStatsPayload{
			Hostname: "testhost",
			Env:      "testing",
			Version:  "0.1-alpha",
			Stats: []pb.ClientStatsBucket{
				{
					Start:    1,
					Duration: 2,
					Stats: []pb.ClientGroupedStats{
						{
							Service:        "",
							Name:           "___noname00___",
							Resource:       "",
							HTTPStatusCode: 200,
							Type:           "web",
							Synthetics:     true,
							Hits:           22,
							Errors:         33,
							Duration:       44,
							OkSummary:      getEmptyDDSketch(),
							ErrorSummary:   getEmptyDDSketch(),
						},
					},
				},
			},
		},
		Out: pb.StatsPayload{
			Stats: []pb.ClientStatsPayload{{
				Hostname: "testhost",
				Env:      "testing",
				Version:  "0.1-alpha",
				Stats: []pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []pb.ClientGroupedStats{
							{
								Service:        "",
								Name:           "___noname00___",
								Resource:       "",
								HTTPStatusCode: 200,
								Type:           "web",
								Synthetics:     true,
								Hits:           22,
								Errors:         33,
								Duration:       44,
								OkSummary:      getEmptyDDSketch(),
								ErrorSummary:   getEmptyDDSketch(),
							},
						},
					},
				},
			},
			},
		},
	},
	{
		In: pb.ClientStatsPayload{
			Hostname: "testhost",
			Env:      "testing",
			Version:  "0.1-alpha",
			Stats: []pb.ClientStatsBucket{
				{
					Start:    1,
					Duration: 2,
					Stats: []pb.ClientGroupedStats{
						{
							Service:        "svc",
							Name:           "noname00",
							Resource:       "/rsc/path",
							HTTPStatusCode: 200,
							Type:           "web",
							DBType:         "",
							Hits:           22,
							Errors:         33,
							Duration:       44,
							OkSummary:      getEmptyDDSketch(),
							ErrorSummary:   getEmptyDDSketch(),
						},
						{
							Service:      "users-db",
							Name:         "sql.query",
							Resource:     "SELECT * FROM users WHERE id=4 AND name='John'",
							Type:         "sql",
							DBType:       "mysql",
							Hits:         5,
							Errors:       7,
							Duration:     8,
							OkSummary:    getEmptyDDSketch(),
							ErrorSummary: getEmptyDDSketch(),
						},
					},
				},
				{
					Start:    3,
					Duration: 4,
					Stats: []pb.ClientGroupedStats{
						{
							Service:      "profiles-db",
							Name:         "sql.query",
							Resource:     "SELECT * FROM profiles WHERE name='Mary'",
							Type:         "sql",
							DBType:       "oracle",
							Hits:         11,
							Errors:       12,
							Duration:     13,
							OkSummary:    getEmptyDDSketch(),
							ErrorSummary: getEmptyDDSketch(),
						},
					},
				},
			},
		},
		Out: pb.StatsPayload{
			Stats: []pb.ClientStatsPayload{{
				Hostname: "testhost",
				Env:      "testing",
				Version:  "0.1-alpha",
				Stats: []pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []pb.ClientGroupedStats{
							{
								Service:        "svc",
								Name:           "noname00",
								Resource:       "/rsc/path",
								HTTPStatusCode: 200,
								Type:           "web",
								DBType:         "",
								Hits:           22,
								Errors:         33,
								Duration:       44,
								OkSummary:      getEmptyDDSketch(),
								ErrorSummary:   getEmptyDDSketch(),
							},
							{
								Service:      "users-db",
								Name:         "sql.query",
								Resource:     "SELECT * FROM users WHERE id=4 AND name='John'",
								Type:         "sql",
								DBType:       "mysql",
								Hits:         5,
								Errors:       7,
								Duration:     8,
								OkSummary:    getEmptyDDSketch(),
								ErrorSummary: getEmptyDDSketch(),
							},
						},
					},
					{
						Start:    3,
						Duration: 4,
						Stats: []pb.ClientGroupedStats{
							{
								Service:      "profiles-db",
								Name:         "sql.query",
								Resource:     "SELECT * FROM profiles WHERE name='Mary'",
								Type:         "sql",
								DBType:       "oracle",
								Hits:         11,
								Errors:       12,
								Duration:     13,
								OkSummary:    getEmptyDDSketch(),
								ErrorSummary: getEmptyDDSketch(),
							},
						},
					},
				},
			},
			},
		},
	},
}
