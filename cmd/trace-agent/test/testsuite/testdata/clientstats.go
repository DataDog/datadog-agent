// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testdata

import (
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/golang/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func getEmptyDDSketch() []byte {
	m, _ := mapping.NewLogarithmicMapping(0.01)
	s := ddsketch.NewDDSketch(m, store.NewDenseStore(), store.NewDenseStore())
	data, _ := proto.Marshal(s.ToProto())
	return data
}

// ClientStatsTests contains a suite of tests for testing the stats endpoint.
var ClientStatsTests = []struct {
	In  *pb.ClientStatsPayload
	Out []*pb.StatsPayload
}{
	{
		In: &pb.ClientStatsPayload{
			Hostname:  "testhost",
			Env:       "testing",
			Version:   "0.1-alpha",
			RuntimeID: "1",
			Sequence:  2,
			Service:   "test-service",
			Stats: []*pb.ClientStatsBucket{
				{
					Start:    1,
					Duration: 2,
					Stats: []*pb.ClientGroupedStats{
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
		Out: []*pb.StatsPayload{{
			AgentHostname:  "agent-hostname",
			AgentEnv:       "agent-env",
			AgentVersion:   "6.0.0",
			ClientComputed: true,
			Stats: []*pb.ClientStatsPayload{{
				Hostname:      "testhost",
				Env:           "testing",
				Version:       "0.1-alpha",
				Lang:          "go",
				TracerVersion: "0.2.0",
				RuntimeID:     "1",
				Sequence:      2,
				Service:       "test-service",
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    0,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "unnamed-go-service",
								Name:           "noname00",
								Resource:       "noname00",
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
	},
	{
		In: &pb.ClientStatsPayload{
			Hostname:  "testhost",
			Env:       "testing",
			Version:   "0.1-alpha",
			RuntimeID: "1",
			Sequence:  2,
			Service:   "test-service",
			Stats: []*pb.ClientStatsBucket{
				{
					Start:    1,
					Duration: 2,
					Stats: []*pb.ClientGroupedStats{
						{
							Service:        "svc",
							Name:           "noname00",
							Resource:       "/rsc/path",
							HTTPStatusCode: 200,
							Type:           "web",
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
					Stats: []*pb.ClientGroupedStats{
						{
							Service:      "profiles-db",
							Name:         "sql.query",
							Resource:     "SELECT * FROM profiles WHERE name='Mary'",
							Type:         "sql",
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
		Out: []*pb.StatsPayload{
			{
				AgentHostname:  "agent-hostname",
				AgentEnv:       "agent-env",
				AgentVersion:   "6.0.0",
				ClientComputed: true,
				Stats: []*pb.ClientStatsPayload{
					{
						Hostname:         "testhost",
						Env:              "testing",
						Version:          "0.1-alpha",
						Lang:             "go",
						TracerVersion:    "0.2.0",
						RuntimeID:        "1",
						Sequence:         2,
						AgentAggregation: "distributions",
						Service:          "test-service",
						Stats: []*pb.ClientStatsBucket{
							{
								Start:    0,
								Duration: 2,
								Stats: []*pb.ClientGroupedStats{
									{
										Service:        "svc",
										Name:           "noname00",
										Resource:       "/rsc/path",
										HTTPStatusCode: 200,
										Type:           "web",
										Hits:           0,
										Errors:         0,
										Duration:       0,
										OkSummary:      getEmptyDDSketch(),
										ErrorSummary:   getEmptyDDSketch(),
									},
									{
										Service:      "users-db",
										Name:         "sql.query",
										Resource:     "SELECT * FROM users WHERE id = ? AND name = ?",
										Type:         "sql",
										Hits:         0,
										Errors:       0,
										Duration:     0,
										OkSummary:    getEmptyDDSketch(),
										ErrorSummary: getEmptyDDSketch(),
									},
								},
							},
						},
					},
					{
						Hostname:         "testhost",
						Env:              "testing",
						Version:          "0.1-alpha",
						Lang:             "go",
						TracerVersion:    "0.2.0",
						RuntimeID:        "1",
						Sequence:         2,
						AgentAggregation: "distributions",
						Service:          "test-service",
						Stats: []*pb.ClientStatsBucket{
							{
								Start:    0,
								Duration: 4,
								Stats: []*pb.ClientGroupedStats{
									{
										Service:      "profiles-db",
										Name:         "sql.query",
										Resource:     "SELECT * FROM profiles WHERE name = ?",
										Type:         "sql",
										Hits:         0,
										Errors:       0,
										Duration:     0,
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
	},
}
