package testdata

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

// ClientStatsTests contains a suite of tests for testing the stats endpoint.
var ClientStatsTests = []struct {
	In  pb.ClientStatsPayload
	Out stats.Payload
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
							Hits:           22,
							Errors:         33,
							Duration:       44,
						},
					},
				},
			},
		},
		Out: stats.Payload{
			HostName: "testhost",
			Env:      "testing",
			Stats: []stats.Bucket{
				{
					Start:    1,
					Duration: 2,
					Counts: map[string]stats.Count{
						"noname00|duration|env:testing,resource:noname00,service:unnamed-service,http.status_code:200,version:0.1-alpha": {
							Key:     "noname00|duration|env:testing,resource:noname00,service:unnamed-service,http.status_code:200,version:0.1-alpha",
							Name:    "noname00",
							Measure: "duration",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "noname00"},
								stats.Tag{Name: "service", Value: "unnamed-service"},
								stats.Tag{Name: "http.status_code", Value: "200"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 22,
							Value:    44,
						},
						"noname00|errors|env:testing,resource:noname00,service:unnamed-service,http.status_code:200,version:0.1-alpha": {
							Key:     "noname00|errors|env:testing,resource:noname00,service:unnamed-service,http.status_code:200,version:0.1-alpha",
							Name:    "noname00",
							Measure: "errors",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "noname00"},
								stats.Tag{Name: "service", Value: "unnamed-service"},
								stats.Tag{Name: "http.status_code", Value: "200"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 22,
							Value:    33,
						},
						"noname00|hits|env:testing,resource:noname00,service:unnamed-service,http.status_code:200,version:0.1-alpha": {
							Key:     "noname00|hits|env:testing,resource:noname00,service:unnamed-service,http.status_code:200,version:0.1-alpha",
							Name:    "noname00",
							Measure: "hits",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "noname00"},
								stats.Tag{Name: "service", Value: "unnamed-service"},
								stats.Tag{Name: "http.status_code", Value: "200"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 22,
							Value:    22,
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
						},
						{
							Service:  "users-db",
							Name:     "sql.query",
							Resource: "SELECT * FROM users WHERE id=4 AND name='John'",
							Type:     "sql",
							DBType:   "mysql",
							Hits:     5,
							Errors:   7,
							Duration: 8,
						},
					},
				},
				{
					Start:    3,
					Duration: 4,
					Stats: []pb.ClientGroupedStats{
						{
							Service:  "profiles-db",
							Name:     "sql.query",
							Resource: "SELECT * FROM profiles WHERE name='Mary'",
							Type:     "sql",
							DBType:   "oracle",
							Hits:     11,
							Errors:   12,
							Duration: 13,
						},
					},
				},
			},
		},
		Out: stats.Payload{
			HostName: "testhost",
			Env:      "testing",
			Stats: []stats.Bucket{
				{
					Start:    1,
					Duration: 2,
					Counts: map[string]stats.Count{
						"noname00|duration|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha": {
							Key:     "noname00|duration|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha",
							Name:    "noname00",
							Measure: "duration",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "/rsc/path"},
								stats.Tag{Name: "service", Value: "svc"},
								stats.Tag{Name: "http.status_code", Value: "200"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 22,
							Value:    44,
						},
						"noname00|errors|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha": {
							Key:     "noname00|errors|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha",
							Name:    "noname00",
							Measure: "errors",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "/rsc/path"},
								stats.Tag{Name: "service", Value: "svc"},
								stats.Tag{Name: "http.status_code", Value: "200"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 22,
							Value:    33,
						},
						"noname00|hits|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha": {
							Key:     "noname00|hits|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha",
							Name:    "noname00",
							Measure: "hits",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "/rsc/path"},
								stats.Tag{Name: "service", Value: "svc"},
								stats.Tag{Name: "http.status_code", Value: "200"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 22,
							Value:    22,
						},
					},
				},
				{
					Start:    1,
					Duration: 2,
					Counts: map[string]stats.Count{
						"sql.query|duration|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha": {
							Key:     "sql.query|duration|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha",
							Name:    "sql.query",
							Measure: "duration",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "SELECT * FROM users WHERE id = ? AND name = ?"},
								stats.Tag{Name: "service", Value: "users-db"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 5,
							Value:    8,
						},
						"sql.query|errors|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha": {
							Key:     "sql.query|errors|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha",
							Name:    "sql.query",
							Measure: "errors",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "SELECT * FROM users WHERE id = ? AND name = ?"},
								stats.Tag{Name: "service", Value: "users-db"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 5,
							Value:    7,
						},
						"sql.query|hits|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha": {
							Key:     "sql.query|hits|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha",
							Name:    "sql.query",
							Measure: "hits",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "SELECT * FROM users WHERE id = ? AND name = ?"},
								stats.Tag{Name: "service", Value: "users-db"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 5,
							Value:    5,
						},
					},
				},
				{
					Start:    3,
					Duration: 4,
					Counts: map[string]stats.Count{
						"sql.query|duration|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha": {
							Key:     "sql.query|duration|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha",
							Name:    "sql.query",
							Measure: "duration",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "SELECT * FROM profiles WHERE name = ?"},
								stats.Tag{Name: "service", Value: "profiles-db"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 11,
							Value:    13,
						},
						"sql.query|errors|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha": {
							Key:     "sql.query|errors|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha",
							Name:    "sql.query",
							Measure: "errors",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "SELECT * FROM profiles WHERE name = ?"},
								stats.Tag{Name: "service", Value: "profiles-db"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 11,
							Value:    12,
						},
						"sql.query|hits|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha": {
							Key:     "sql.query|hits|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha",
							Name:    "sql.query",
							Measure: "hits",
							TagSet: stats.TagSet{
								stats.Tag{Name: "env", Value: "testing"},
								stats.Tag{Name: "resource", Value: "SELECT * FROM profiles WHERE name = ?"},
								stats.Tag{Name: "service", Value: "profiles-db"},
								stats.Tag{Name: "version", Value: "0.1-alpha"},
							},
							TopLevel: 11,
							Value:    11,
						},
					},
				},
			},
		},
	},
}
