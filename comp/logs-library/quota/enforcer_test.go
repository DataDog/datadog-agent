// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quota

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

var t0 = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func newTestEnforcer(now *time.Time) *Enforcer {
	e := NewEnforcer()
	e.now = func() time.Time { return *now }
	return e
}

func newMsg(source, service, status string, tags ...string) *message.Message {
	src := sources.NewLogSource("", &config.LogsConfig{Source: source, Service: service, Tags: tags})
	return message.NewMessageWithSource([]byte("hello"), status, src, 0)
}

func rule(id string, version int64, match string, extra string) []byte {
	return fmt.Appendf(nil, `{"schema_version":1,"quota_id":%q,"version":%d,"action":"drop","match":%s,"max_ttl_seconds":3600%s}`,
		id, version, match, extra)
}

func TestCheckNoRules(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	_, drop := e.Check(newMsg("nginx", "web", message.StatusInfo))
	assert.False(t, drop)
}

func TestMatchFields(t *testing.T) {
	tests := []struct {
		name  string
		match string
		msg   *message.Message
		drop  bool
	}{
		{"source eq", `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`, newMsg("nginx", "web", ""), true},
		{"source miss", `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`, newMsg("redis", "web", ""), false},
		{"service in", `{"all":[{"field":"service","op":"in","values":["a","web"]}]}`, newMsg("nginx", "web", ""), true},
		{"status default info", `{"all":[{"field":"status","op":"eq","value":"info"}]}`, newMsg("nginx", "web", ""), true},
		{"status miss", `{"all":[{"field":"status","op":"eq","value":"error"}]}`, newMsg("nginx", "web", message.StatusWarning), false},
		{"tag", `{"all":[{"field":"tag","op":"eq","value":"team:payments"}]}`, newMsg("nginx", "web", "", "env:prod", "team:payments"), true},
		{"tag miss", `{"all":[{"field":"tag","op":"eq","value":"team:payments"}]}`, newMsg("nginx", "web", "", "team:search"), false},
		{"all requires every clause", `{"all":[{"field":"source","op":"eq","value":"nginx"},{"field":"service","op":"eq","value":"api"}]}`, newMsg("nginx", "web", ""), false},
		{"any", `{"any":[{"field":"service","op":"eq","value":"api"},{"field":"source","op":"eq","value":"nginx"}]}`, newMsg("nginx", "web", ""), true},
		{"all and any", `{"all":[{"field":"source","op":"eq","value":"nginx"}],"any":[{"field":"service","op":"eq","value":"api"}]}`, newMsg("nginx", "web", ""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := t0
			e := newTestEnforcer(&now)
			res := e.Update("mock", map[string][]byte{"q": rule("q1", 1, tt.match, "")})
			require.NoError(t, res["q"])
			id, drop := e.Check(tt.msg)
			assert.Equal(t, tt.drop, drop)
			if drop {
				assert.Equal(t, "q1", id)
			}
		})
	}
}

func TestHostTags(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.SetHostTagsProvider(func() []string { return []string{"team:payments", "env:prod"} })

	e.Update("mock", map[string][]byte{
		"hit":      rule("hit", 1, `{"all":[{"field":"host_tag","op":"eq","value":"team:payments"},{"field":"source","op":"eq","value":"nginx"}]}`, ""),
		"miss":     rule("miss", 1, `{"all":[{"field":"host_tag","op":"eq","value":"team:search"}]}`, ""),
		"any-hit":  rule("any-hit", 1, `{"any":[{"field":"host_tag","op":"eq","value":"env:prod"},{"field":"service","op":"eq","value":"nope"}]}`, ""),
		"any-miss": rule("any-miss", 1, `{"any":[{"field":"host_tag","op":"eq","value":"env:dev"}]}`, ""),
	})

	_, states := e.Status()
	got := map[string]string{}
	for _, s := range states {
		got[s.Path] = s.State
	}
	assert.Equal(t, map[string]string{
		"hit": "enforcing", "miss": "not applicable (host tags)",
		"any-hit": "enforcing", "any-miss": "not applicable (host tags)",
	}, got)

	id, drop := e.Check(newMsg("redis", "web", ""))
	assert.True(t, drop)
	assert.Equal(t, "any-hit", id)
}

func TestInvalidConfigs(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	src := `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`
	res := e.Update("mock", map[string][]byte{
		"json":    []byte(`{`),
		"schema":  []byte(`{"schema_version":2,"quota_id":"a","action":"drop","match":` + src + `,"max_ttl_seconds":1}`),
		"action":  []byte(`{"schema_version":1,"quota_id":"a","action":"sample","match":` + src + `,"max_ttl_seconds":1}`),
		"noexp":   []byte(`{"schema_version":1,"quota_id":"a","action":"drop","match":` + src + `}`),
		"nomatch": rule("a", 1, `{}`, ""),
		"field":   rule("a", 1, `{"all":[{"field":"team","op":"eq","value":"x"}]}`, ""),
		"op":      rule("a", 1, `{"all":[{"field":"source","op":"regex","value":"x"}]}`, ""),
		"unknown": rule("ok", 1, src, `,"future_field":true`),
	})
	for path, err := range res {
		if path == "unknown" {
			assert.NoError(t, err, path)
		} else {
			assert.Error(t, err, path)
		}
	}
	id, drop := e.Check(newMsg("nginx", "", ""))
	assert.True(t, drop)
	assert.Equal(t, "ok", id)
}

func TestExpiry(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	src := `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`
	until := t0.Add(10 * time.Minute).Format(time.RFC3339)
	e.Update("mock", map[string][]byte{
		"ttl":     rule("ttl", 1, src, ""),
		"a-until": []byte(`{"schema_version":1,"quota_id":"until","version":1,"action":"drop","match":` + src + `,"enforce_until":"` + until + `","max_ttl_seconds":86400}`),
	})
	msg := newMsg("nginx", "", "")

	id, drop := e.Check(msg)
	assert.True(t, drop)
	assert.Equal(t, "until", id)

	now = t0.Add(30 * time.Minute)
	id, drop = e.Check(msg)
	assert.True(t, drop)
	assert.Equal(t, "ttl", id)

	now = t0.Add(time.Hour)
	_, drop = e.Check(msg)
	assert.False(t, drop)

	// Re-sending the same config must not restart the TTL.
	e.Update("mock", map[string][]byte{"ttl": rule("ttl", 1, src, "")})
	_, drop = e.Check(msg)
	assert.False(t, drop)

	// A new version does.
	e.Update("mock", map[string][]byte{"ttl": rule("ttl", 2, src, "")})
	_, drop = e.Check(msg)
	assert.True(t, drop)
}

func TestVersions(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	nginx := `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`
	redis := `{"all":[{"field":"source","op":"eq","value":"redis"}]}`

	e.Update("mock", map[string][]byte{"q": rule("q", 5, nginx, "")})
	res := e.Update("mock", map[string][]byte{"q": rule("q", 4, redis, "")})
	assert.ErrorContains(t, res["q"], "stale version")
	_, drop := e.Check(newMsg("redis", "", ""))
	assert.False(t, drop)

	// Same quota from two sources: the newer version wins.
	e.Update("mock", map[string][]byte{"q": rule("q", 6, nginx, "")})
	e.Update("rc", map[string][]byte{"q": rule("q", 7, redis, "")})
	_, drop = e.Check(newMsg("redis", "", ""))
	assert.True(t, drop)
	_, drop = e.Check(newMsg("nginx", "", ""))
	assert.False(t, drop)
}

func TestUpdateRemovesOnlyOwnSource(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update("mock", map[string][]byte{"a": rule("a", 1, `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`, "")})
	e.Update("rc", map[string][]byte{"b": rule("b", 1, `{"all":[{"field":"source","op":"eq","value":"redis"}]}`, "")})

	e.Update("rc", map[string][]byte{})
	_, drop := e.Check(newMsg("redis", "", ""))
	assert.False(t, drop)
	_, drop = e.Check(newMsg("nginx", "", ""))
	assert.True(t, drop)
}

func TestStats(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update("mock", map[string][]byte{"q": rule("q", 1, `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`, "")})
	for range 3 {
		e.Check(newMsg("nginx", "", ""))
	}
	e.Check(newMsg("redis", "", ""))

	lastSync, states := e.Status()
	assert.NotEmpty(t, lastSync)
	require.Len(t, states, 1)
	assert.EqualValues(t, 3, states[0].DroppedEvents)
	assert.EqualValues(t, 15, states[0].DroppedBytes)
	assert.Equal(t, "all(source in [nginx])", states[0].Match)
}

func TestConcurrentCheckAndUpdate(_ *testing.T) {
	e := NewEnforcer()
	msg := newMsg("nginx", "", "")
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					e.Check(msg)
				}
			}
		}()
	}
	for i := range 200 {
		e.Update("rc", map[string][]byte{"q": rule("q", int64(i), `{"all":[{"field":"source","op":"eq","value":"nginx"}]}`, "")})
		e.Status()
	}
	close(stop)
	wg.Wait()
}

func BenchmarkCheck(b *testing.B) {
	msg := newMsg("nginx", "web", "", "env:prod", "team:search")
	b.Run("no rules", func(b *testing.B) {
		e := NewEnforcer()
		for b.Loop() {
			e.Check(msg)
		}
	})
	b.Run("tag rule miss", func(b *testing.B) {
		e := NewEnforcer()
		e.Update("rc", map[string][]byte{"q": rule("q", 1, `{"all":[{"field":"tag","op":"eq","value":"team:payments"}]}`, "")})
		for b.Loop() {
			e.Check(msg)
		}
	})
}
