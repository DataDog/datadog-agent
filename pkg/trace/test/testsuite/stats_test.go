package testsuite

import (
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
)

func TestClientStats(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	t.Run("ok", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		if err := r.PostMsgpack("/v0.5/stats", &gotClientStats); err != nil {
			t.Fatal(err)
		}
		timeout := time.After(3 * time.Second)
		out := r.Out()
		for {
			select {
			case p := <-out:
				got, ok := p.(stats.Payload)
				if !ok {
					continue
				}
				if !reflect.DeepEqual(got, wantClientStats) {
					t.Fatal("did not match")
				} else {
					return
				}
			case <-timeout:
				t.Fatalf("timed out, log was:\n%s", r.AgentLog())
			}
		}
	})
}

var gotClientStats = pb.ClientStatsPayload{
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
}

var wantClientStats = stats.Payload{
	HostName: "testhost",
	Env:      "testing",
	Stats: []stats.Bucket{
		stats.Bucket{
			Start:    1,
			Duration: 2,
			Counts: map[string]stats.Count{
				"noname00|duration|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha": stats.Count{
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
				"noname00|errors|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha": stats.Count{
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
				"noname00|hits|env:testing,resource:/rsc/path,service:svc,http.status_code:200,version:0.1-alpha": stats.Count{
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
		stats.Bucket{
			Start:    1,
			Duration: 2,
			Counts: map[string]stats.Count{
				"sql.query|duration|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha": stats.Count{
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
				"sql.query|errors|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha": stats.Count{
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
				"sql.query|hits|env:testing,resource:SELECT * FROM users WHERE id = ? AND name = ?,service:users-db,version:0.1-alpha": stats.Count{
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
		stats.Bucket{
			Start:    3,
			Duration: 4,
			Counts: map[string]stats.Count{
				"sql.query|duration|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha": stats.Count{
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
				"sql.query|errors|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha": stats.Count{
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
				"sql.query|hits|env:testing,resource:SELECT * FROM profiles WHERE name = ?,service:profiles-db,version:0.1-alpha": stats.Count{
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
}
