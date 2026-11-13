// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quota

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func withHostname(m *message.Message, hostname string) *message.Message {
	m.Hostname = hostname
	return m
}

func hostRuleSet(t *testing.T, hostname string, hostTags []string, tagLists ...[]string) *ruleSet {
	e := NewEnforcer()
	e.tickEvery = 0
	e.SetHostTags(hostTags, hostname)
	quotas, err := parseSnapshot(snap(t0.Add(time.Hour), tagLists...))
	require.NoError(t, err)
	return newRuleSet(quotas, t0, e.hostname, e.hostTags)
}

func TestHostTagsDocExample(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.SetHostTags([]string{"env:prod", "team:payments"}, "web-1")
	e.Update(map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"env:prod", "service:checkout"})})

	checkout := func(hostname string) *message.Message {
		src := sources.NewLogSource("", &config.LogsConfig{Service: "checkout"})
		return withHostname(message.NewMessageWithSource([]byte("x"), "", src, 0), hostname)
	}
	cart := sources.NewLogSource("", &config.LogsConfig{Service: "cart"})

	assert.True(t, e.Check(checkout("")))
	assert.True(t, e.Check(checkout("WEB-1")))
	assert.False(t, e.Check(checkout("router-7")))
	assert.False(t, e.Check(message.NewMessageWithSource([]byte("x"), "", cart, 0)))
}

func TestHostTagMatching(t *testing.T) {
	host := []string{"Env:Prod", "team:payments", "team:payments", "region:us"}
	tests := []struct {
		name     string
		quotas   [][]string
		logTags  []string
		hostname string
		exclude  []string
		want     int
	}{
		{"host-only quota", [][]string{{"env:prod"}}, nil, "", nil, 0},
		{"host-only, case-insensitive", [][]string{{"ENV:prod", "Team:Payments"}}, nil, "", nil, 0},
		{"host-only, foreign host", [][]string{{"env:prod"}}, nil, "other", nil, -1},
		{"foreign host uses its own tags", [][]string{{"env:prod"}}, []string{"env:prod"}, "other", nil, 0},
		{"host-only, local hostname", [][]string{{"env:prod"}}, nil, "My-Host", nil, 0},
		{"partial, log supplies the rest", [][]string{{"env:prod", "app:x"}}, []string{"app:x"}, "", nil, 0},
		{"partial, log lacks the rest", [][]string{{"env:prod", "app:x"}}, []string{"app:y"}, "", nil, -1},
		{"disjoint", [][]string{{"env:staging"}}, nil, "", nil, -1},
		{"disjoint, log supplies it", [][]string{{"env:staging"}}, []string{"env:staging"}, "", nil, 0},
		{"filter ignored for host tags", [][]string{{"env:prod", "app:x"}}, []string{"app:x"}, "", []string{"env:*"}, 0},
		{"filter applies to the rest", [][]string{{"env:prod", "app:x"}}, []string{"app:x"}, "", []string{"app:*"}, -1},
		{"filter drops log's own env on a foreign host", [][]string{{"env:prod"}}, []string{"env:prod"}, "other", []string{"env:*"}, -1},
		{"earlier partial beats later host-only", [][]string{{"env:prod", "app:x"}, {"region:us"}}, []string{"app:x"}, "", nil, 0},
		{"later host-only when partial misses", [][]string{{"env:prod", "app:x"}, {"region:us"}}, nil, "", nil, 1},
		{"no-tag quota still matches everything", [][]string{{"env:staging"}, {}}, nil, "other", nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := hostRuleSet(t, "my-host", host, tt.quotas...)
			src := newSnapshotSource(nil, nil, "")
			if tt.exclude != nil {
				setFilter(t, src.source, nil, tt.exclude)
			}
			var want *quota
			if tt.want >= 0 {
				want = rs.quotas[tt.want]
			}
			assert.Same(t, want, rs.match(src.msg(nil, tt.logTags).Origin, tt.hostname), "snapshot")
			assert.Same(t, want, rs.match(src.fallbackMsg(nil, tt.logTags).Origin, tt.hostname), "fallback")
		})
	}
}

func TestNoHostTagsHasNoHostView(t *testing.T) {
	rs := hostRuleSet(t, "my-host", nil, []string{"env:prod"})
	assert.Nil(t, rs.local)
	rs = hostRuleSet(t, "my-host", []string{"", ""}, []string{"env:prod"})
	assert.Nil(t, rs.local)
}

func TestHostViewFull(t *testing.T) {
	rs := hostRuleSet(t, "h", []string{"env:prod"}, []string{"app:x"}, []string{"env:prod"}, []string{})
	assert.Equal(t, 1, rs.local.full)
	assert.Equal(t, [][]string{{"app:x"}, {}, {}}, rs.local.residual)

	rs = hostRuleSet(t, "h", []string{"env:prod"}, []string{"env:prod"})
	assert.Equal(t, 0, rs.local.full)
	assert.Same(t, rs.quotas[0], rs.match(nil, ""))
}

// hostReference is the matching rule written out directly: every quota tag is a host tag on a local
// log, or one of the log's transport tags, payload attribute tags or service.
func hostReference(rs *ruleSet, o *message.Origin, logHostname, agentHostname string, hostTags []string) *quota {
	local := len(hostTags) > 0 && (logHostname == "" || strings.EqualFold(logHostname, agentHostname))
	have := o.TransportTags()
	for _, t := range o.Tags() {
		if key, _, _ := strings.Cut(t, ":"); tagfilter.IsPayloadAttributeKey(key) {
			have = append(have, t)
		}
	}
	if service := o.Service(); service != "" {
		have = append(have, "service:"+service)
	}
	for _, q := range rs.quotas {
		if !slices.ContainsFunc(q.tags, func(t string) bool {
			if local && slices.ContainsFunc(hostTags, func(h string) bool { return strings.EqualFold(h, t) }) {
				return false
			}
			return !slices.ContainsFunc(have, func(h string) bool { return strings.EqualFold(h, t) })
		}) {
			return q
		}
	}
	return nil
}

func TestHostMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	vocab := []string{"env:prod", "env:dev", "ENV:prod", "team:a", "team:b", "stream:stderr", "app:x", "region:us", "service:web", "service:api", "Host:a", "Team:A", "sourcecategory:cat"}
	filters := [][]string{nil, {"env:*"}, {"team:*", "stream:*"}, {"region:*", "service:*"}}
	hostnames := []string{"", "my-host", "MY-HOST", "other"}
	pick := func(max int) []string {
		var out []string
		for range rng.IntN(max + 1) {
			out = append(out, vocab[rng.IntN(len(vocab))])
		}
		return out
	}
	for round := range 500 {
		var tagLists [][]string
		for range 1 + rng.IntN(8) {
			tagLists = append(tagLists, pick(3))
		}
		hostTags := pick(4)
		rs := hostRuleSet(t, "my-host", hostTags, tagLists...)
		category := ""
		if rng.IntN(2) == 0 {
			category = "cat"
		}
		src := newServiceSource(pick(2), pick(2), category, []string{"", "web", "api"}[rng.IntN(3)])
		if f := filters[rng.IntN(len(filters))]; f != nil {
			setFilter(t, src.source, nil, f)
		}
		for range 20 {
			src.msgService = []string{"", "web", "api"}[rng.IntN(3)]
			dynamic, msgTags := pick(3), pick(2)
			hostname := hostnames[rng.IntN(len(hostnames))]
			msg := src.msg(dynamic, msgTags)
			want := hostReference(rs, msg.Origin, hostname, "my-host", hostTags)
			ctx := fmt.Sprintf("round %d quotas %v host %v hostname %q tags %v", round, tagLists, hostTags, hostname, msg.Origin.Tags())
			require.Same(t, want, rs.match(msg.Origin, hostname), ctx)
			require.Same(t, want, rs.match(msg.Origin, hostname), "cached "+ctx)
			require.Same(t, want, rs.match(src.fallbackMsg(dynamic, msgTags).Origin, hostname), "fallback "+ctx)
		}
	}
}

func TestPlansCachedPerLocality(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.SetHostTags([]string{"env:prod"}, "h")
	e.Update(map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"env:prod", "app:x"})})
	src := newSnapshotSource(nil, []string{"app:x"}, "")
	local, remote := src.msg(nil, nil), withHostname(src.msg(nil, nil), "other")
	shared, _, _, _ := local.Origin.TagSnapshot()

	assert.True(t, e.Check(local))
	assert.False(t, e.Check(remote))
	rs := e.active.Load()
	lp, rp := rs.planFor(shared, nil, true), rs.planFor(shared, nil, false)
	assert.NotSame(t, lp, rp)
	assert.True(t, e.Check(local))
	assert.Same(t, lp, rs.planFor(shared, nil, true), "a remote log must not evict the local plan")
	assert.Same(t, rp, rs.planFor(shared, nil, false))
}

func TestSetHostTagsRebuildsOnlyOnChange(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update(map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"team:a"})})
	src := newSnapshotSource(nil, nil, "")
	msg := src.msg(nil, nil)
	assert.False(t, e.Check(msg))

	e.SetHostTags([]string{"team:a"}, "h")
	rs := e.active.Load()
	assert.True(t, e.Check(msg))

	e.SetHostTags([]string{"TEAM:a", "team:a"}, "h")
	assert.Same(t, rs, e.active.Load(), "equivalent tags keep the rule set and its plans")

	e.SetHostTags(nil, "h")
	assert.NotSame(t, rs, e.active.Load())
	assert.False(t, e.Check(msg), "removed host tags stop matching")

	e.SetHostTags([]string{"team:a"}, "h")
	assert.True(t, e.Check(withHostname(src.msg(nil, nil), "H")))
	e.SetHostTags([]string{"team:a"}, "renamed")
	assert.False(t, e.Check(withHostname(src.msg(nil, nil), "h")), "a hostname change moves which logs are local")
}

func TestSetHostTagsTrimsWhitespace(t *testing.T) {
	e := NewEnforcer()
	e.SetHostTags([]string{" env:prod", "team:a ", "  "}, "h")
	_, tags := e.HostTags()
	assert.Equal(t, []string{"env:prod", "team:a"}, tags)
}

func TestUpdateWithHostTags(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update(map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"team:a"})})
	old := e.active.Load()

	e.UpdateWithHostTags(map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"team:b"})}, []string{"Team:B"}, "h")
	rs := e.active.Load()
	assert.NotSame(t, old, rs)
	hostname, tags := e.HostTags()
	assert.Equal(t, "h", hostname)
	assert.Equal(t, []string{"team:b"}, tags)
	assert.True(t, e.Check(newMsg()), "new quotas match on the host tags set with them")
}

func TestHostTagsSurviveUpdateAndExpiry(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.SetHostTags([]string{"env:prod"}, "h")
	e.Update(map[string][]byte{
		"a": snap(t0.Add(time.Minute), []string{"app:x"}),
		"b": snap(t0.Add(time.Hour), []string{"env:prod"}),
	})
	assert.True(t, e.Check(newMsg()))
	now = t0.Add(2 * time.Minute)
	e.tick()
	require.Len(t, e.active.Load().quotas, 1)
	assert.Equal(t, 0, e.active.Load().local.full)
	assert.True(t, e.Check(newMsg()))
}

func TestStatusHostTags(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.SetHostTags([]string{"Team:Payments", "env:prod"}, "h")
	e.Update(map[string][]byte{"c": snap(t0.Add(time.Hour), []string{"env:prod", "service:checkout"}, []string{"env:dev"})})

	hostname, tags := e.HostTags()
	assert.Equal(t, "h", hostname)
	assert.Equal(t, []string{"env:prod", "team:payments"}, tags)
	_, configs := e.Status()
	require.Len(t, configs[0].Quotas, 2)
	assert.Equal(t, "env:prod", configs[0].Quotas[0].HostTags)
	assert.Empty(t, configs[0].Quotas[1].HostTags)
}

func TestConcurrentSetHostTags(_ *testing.T) {
	e := NewEnforcer()
	src := newSnapshotSource(nil, []string{"app:x"}, "")
	msgs := []*message.Message{src.msg(nil, nil), withHostname(src.msg(nil, nil), "other"), newMsg("app:x")}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for _, msg := range msgs {
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
		e.SetHostTags([]string{"env:" + strconv.Itoa(i%3)}, "h")
		e.Update(map[string][]byte{"c": snap(time.Now().Add(time.Hour), []string{"env:1", "app:x"}, []string{"env:2"})})
		e.Status()
	}
	close(stop)
	wg.Wait()
}

func BenchmarkCheckHostTags(b *testing.B) {
	static := make([]string, 46)
	for i := range static {
		static[i] = "key" + strconv.Itoa(i) + ":value"
	}
	msgTags := []string{"stream:stdout"}
	host := []string{"env:prod", "team:payments"}
	variants := []struct {
		name  string
		host  []string
		front func(n int) [][]string
	}{
		{"no host tags", nil, func(int) [][]string { return nil }},
		{"host tags, none used", host, func(int) [][]string { return nil }},
		{"host-only first", host, func(int) [][]string { return [][]string{{"env:prod"}} }},
		{"partial on snapshot", host, func(int) [][]string { return [][]string{{"env:prod", static[0]}} }},
		{"partial on message", host, func(int) [][]string { return [][]string{{"env:prod", "stream:stdout"}} }},
		{"host-only last", host, func(int) [][]string { return nil }},
	}
	for _, v := range variants {
		for _, n := range []int{0, 1, 10, 100, 500} {
			tagLists := v.front(n)
			for i := range n {
				tagLists = append(tagLists, []string{static[0], "team:t" + strconv.Itoa(i)})
			}
			if v.name == "host-only last" {
				tagLists = append(tagLists, []string{"team:payments"})
			}
			if len(tagLists) == 0 {
				continue
			}
			e := NewEnforcer()
			e.SetHostTags(v.host, "h")
			e.Update(map[string][]byte{"c": snap(time.Now().Add(time.Hour), tagLists...)})
			src := newSnapshotSource(nil, static, "")
			local := src.msg(nil, msgTags)
			remote := withHostname(src.msg(nil, msgTags), "other")
			fallback := src.fallbackMsg(nil, msgTags)
			for name, msg := range map[string]*message.Message{"local": local, "remote": remote, "fallback": fallback} {
				b.Run(fmt.Sprintf("%s/quotas=%d/%s", v.name, n, name), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						e.Check(msg)
					}
				})
			}
			b.Run(fmt.Sprintf("%s/quotas=%d/mixed", v.name, n), func(b *testing.B) {
				b.ReportAllocs()
				i := 0
				for b.Loop() {
					i++
					if i%2 == 0 {
						e.Check(local)
					} else {
						e.Check(remote)
					}
				}
			})
		}
	}
}
