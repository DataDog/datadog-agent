// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quota

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func newTestEnforcer(now *time.Time) *Enforcer {
	e := NewEnforcer()
	e.now = func() time.Time { return *now }
	return e
}

func newMsg(tags ...string) *message.Message {
	src := sources.NewLogSource("", &config.LogsConfig{Source: "nginx", Service: "web", Tags: tags})
	return message.NewMessageWithSource([]byte("hello"), "", src, 0)
}

func snap(expiresAt time.Time, tagLists ...[]string) []byte {
	entries := make([]string, len(tagLists))
	for i, tags := range tagLists {
		quoted := make([]string, len(tags))
		for j, t := range tags {
			quoted[j] = fmt.Sprintf("%q", t)
		}
		entries[i] = fmt.Sprintf(`{"tags":[%s],"expires_at":%d}`, strings.Join(quoted, ","), expiresAt.Unix())
	}
	return []byte(`{"quotas":[` + strings.Join(entries, ",") + `]}`)
}

func TestCheckNoQuotas(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	assert.False(t, e.Check(newMsg("env:prod")))

	e.Update(map[string][]byte{"c": []byte(`{"quotas":[]}`)})
	assert.False(t, e.Check(newMsg("env:prod")))
}

func TestMatchTags(t *testing.T) {
	tests := []struct {
		name  string
		quota []string
		msg   *message.Message
		drop  bool
	}{
		{"single tag", []string{"env:prod"}, newMsg("env:prod", "team:a"), true},
		{"tag miss", []string{"env:prod"}, newMsg("env:dev"), false},
		{"all tags required", []string{"env:prod", "team:a"}, newMsg("env:prod", "team:b"), false},
		{"all tags present", []string{"team:a", "env:prod"}, newMsg("env:prod", "team:a"), true},
		{"no tags drops everything", nil, newMsg(), true},
		{"service is not a tag", []string{"service:web"}, newMsg("env:prod"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := t0
			e := newTestEnforcer(&now)
			require.NoError(t, e.Update(map[string][]byte{"c": snap(t0.Add(time.Hour), tt.quota)})["c"])
			assert.Equal(t, tt.drop, e.Check(tt.msg))
		})
	}
}

func TestInvalidConfigs(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	exp := strconv.FormatInt(t0.Add(time.Hour).Unix(), 10)
	res := e.Update(map[string][]byte{
		"json":      []byte(`{`),
		"noquotas":  []byte(`{"hello":"world"}`),
		"noexpiry":  []byte(`{"quotas":[{"tags":["env:prod"]}]}`),
		"emptytag":  []byte(`{"quotas":[{"tags":[""],"expires_at":1}]}`),
		"unknown":   []byte(`{"quotas":[{"tags":["env:prod"],"expires_at":` + exp + `,"future":1}],"other":true}`),
		"oneBadAll": []byte(`{"quotas":[{"tags":["team:a"],"expires_at":` + exp + `},{"tags":["team:b"]}]}`),
	})
	for path, err := range res {
		if path == "unknown" {
			assert.NoError(t, err, path)
		} else {
			assert.Error(t, err, path)
		}
	}
	assert.True(t, e.Check(newMsg("env:prod")))
	assert.False(t, e.Check(newMsg("team:a")))
}

func TestExpiry(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update(map[string][]byte{
		"past":   snap(t0.Add(-time.Minute), []string{"team:a"}),
		"future": snap(t0.Add(10*time.Minute), []string{"team:b"}),
	})

	assert.False(t, e.Check(newMsg("team:a")), "already expired on receipt")
	assert.True(t, e.Check(newMsg("team:b")))

	now = t0.Add(10 * time.Minute)
	assert.False(t, e.Check(newMsg("team:b")), "stops at expires_at without a new config")

	_, configs := e.Status()
	require.Len(t, configs, 2)
	assert.Equal(t, "expired", configs[0].Quotas[0].State)
	assert.Equal(t, t0.Add(10*time.Minute).Format(time.RFC3339), configs[0].Quotas[0].ExpiresAt)
	assert.Equal(t, "expired", configs[1].Quotas[0].State)
}

func TestExpiredQuotasArePruned(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update(map[string][]byte{
		"a": snap(t0.Add(time.Minute), []string{"team:a"}),
		"b": snap(t0.Add(time.Hour), []string{"team:b"}),
	})
	assert.Len(t, e.active.Load().quotas, 2)

	now = t0.Add(time.Minute)
	assert.False(t, e.Check(newMsg("team:a")))
	assert.Len(t, e.active.Load().quotas, 1)
	assert.True(t, e.Check(newMsg("team:b")))

	now = t0.Add(time.Hour)
	assert.False(t, e.Check(newMsg("team:b")))
	assert.Empty(t, e.active.Load().quotas)
}

func TestTelemetrySeriesRemovedWithQuota(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update(map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"team:tlm"})})
	e.Check(newMsg("team:tlm"))
	assert.EqualValues(t, 1, metrics.TlmQuotaDroppedEvents.WithValues("team:tlm").Get())

	e.Update(map[string][]byte{})
	assert.Zero(t, metrics.TlmQuotaDroppedEvents.WithValues("team:tlm").Get())
}

func TestUpdateReplacesConfigs(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	exp := t0.Add(time.Hour)
	e.Update(map[string][]byte{"a": snap(exp, []string{"team:a"}), "b": snap(exp, []string{"team:b"})})
	e.Update(map[string][]byte{"b": []byte(`{"quotas":[]}`)})

	assert.False(t, e.Check(newMsg("team:a")))
	assert.False(t, e.Check(newMsg("team:b")))
	_, configs := e.Status()
	require.Len(t, configs, 1)
	assert.Empty(t, configs[0].Quotas)
}

func TestStats(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	cfg := map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"team:a", "env:prod"})}
	e.Update(cfg)
	for range 3 {
		e.Check(newMsg("env:prod", "team:a"))
	}
	e.Check(newMsg("team:b"))
	e.Update(cfg)

	lastSync, configs := e.Status()
	assert.NotEmpty(t, lastSync)
	require.Len(t, configs, 1)
	require.Len(t, configs[0].Quotas, 1)
	q := configs[0].Quotas[0]
	assert.Equal(t, "env:prod,team:a", q.Tags)
	assert.Equal(t, "enforcing", q.State)
	assert.EqualValues(t, 3, q.DroppedEvents, "counts survive a re-sent config")
	assert.EqualValues(t, 15, q.DroppedBytes)
}

func TestConcurrentCheckAndUpdate(_ *testing.T) {
	e := NewEnforcer()
	msg := newMsg("team:a")
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
		e.Update(map[string][]byte{"c": snap(time.Now().Add(time.Duration(i)*time.Second), []string{"team:a"})})
		e.Status()
	}
	close(stop)
	wg.Wait()
}

func BenchmarkCheck(b *testing.B) {
	msg := newMsg("env:prod", "team:search")
	b.Run("no quotas", func(b *testing.B) {
		e := NewEnforcer()
		for b.Loop() {
			e.Check(msg)
		}
	})
	b.Run("tag miss", func(b *testing.B) {
		e := NewEnforcer()
		e.Update(map[string][]byte{"c": snap(time.Now().Add(time.Hour), []string{"team:payments"})})
		for b.Loop() {
			e.Check(msg)
		}
	})
	b.Run("tag hit", func(b *testing.B) {
		e := NewEnforcer()
		e.Update(map[string][]byte{"c": snap(time.Now().Add(time.Hour), []string{"env:prod", "team:search"})})
		for b.Loop() {
			e.Check(msg)
		}
	})
	b.Run("50 quotas miss", func(b *testing.B) {
		e := NewEnforcer()
		e.Update(manyQuotas(50, time.Now().Add(time.Hour)))
		for b.Loop() {
			e.Check(msg)
		}
	})
	b.Run("50 quotas expired", func(b *testing.B) {
		e := NewEnforcer()
		e.Update(manyQuotas(50, time.Now().Add(time.Second)))
		e.now = func() time.Time { return time.Now().Add(time.Hour) }
		for b.Loop() {
			e.Check(msg)
		}
	})
}

func manyQuotas(n int, expiresAt time.Time) map[string][]byte {
	configs := make(map[string][]byte, n)
	for i := range n {
		configs[strconv.Itoa(i)] = snap(expiresAt, []string{"team:t" + strconv.Itoa(i)})
	}
	return configs
}
