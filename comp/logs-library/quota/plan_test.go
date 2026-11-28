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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type snapshotSource struct {
	source      *sources.LogSource
	static      []string
	snapshotter message.TagSnapshotter
	msgService  string
}

func newSnapshotSource(configTags, static []string, category string) *snapshotSource {
	return newServiceSource(configTags, static, category, "")
}

func newServiceSource(configTags, static []string, category, service string) *snapshotSource {
	return &snapshotSource{
		source: sources.NewLogSource("", &config.LogsConfig{Tags: configTags, SourceCategory: category, Service: service}),
		static: static,
	}
}

func (s *snapshotSource) msg(dynamic, msgTags []string) *message.Message {
	origin := message.NewOrigin(s.source)
	tags := slices.Concat(s.static, dynamic, msgTags)
	origin.SetTagsWithSnapshot(tags, s.snapshotter.Get(s.source, s.static, dynamic), msgTags)
	origin.SetService(s.msgService)
	return message.NewMessage([]byte("hello"), origin, "", 0)
}

func (s *snapshotSource) fallbackMsg(dynamic, msgTags []string) *message.Message {
	origin := message.NewOrigin(s.source)
	origin.SetTags(slices.Concat(s.static, dynamic, msgTags))
	origin.SetService(s.msgService)
	return message.NewMessage([]byte("hello"), origin, "", 0)
}

func setFilter(t testing.TB, src *sources.LogSource, include, exclude []string) {
	f, report := tagfilter.Compile(include, exclude)
	require.Empty(t, report.Rejected)
	if scoped := tagfilter.NewScoped(f, nil); scoped != nil {
		require.True(t, src.SetTagFilterIfUnset(scoped))
	}
}

func transportMatch(rs *ruleSet, o *message.Origin) *quota {
	tags := o.TransportTags()
	for _, t := range o.Tags() {
		if key, _, _ := strings.Cut(t, ":"); tagfilter.IsPayloadAttributeKey(key) {
			tags = append(tags, t)
		}
	}
	if service := o.Service(); service != "" {
		tags = append(tags, "service:"+service)
	}
	for _, q := range rs.quotas {
		if !slices.ContainsFunc(q.tags, func(t string) bool {
			return !slices.ContainsFunc(tags, func(have string) bool { return equalFold(have, t) })
		}) {
			return q
		}
	}
	return nil
}

func ruleSetOf(t *testing.T, tagLists ...[]string) *ruleSet {
	quotas, err := parseSnapshot(snap(t0.Add(time.Hour), tagLists...))
	require.NoError(t, err)
	return newRuleSet(quotas, t0)
}

func TestPlanMatchesLinear(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	vocab := []string{"env:prod", "env:dev", "ENV:prod", "team:a", "team:b", "stream:stderr", "stream:stdout", "app:x", "sourcecategory:cat", "sourcecategory:other", "region:us", "service:web", "service:api", "Service:WEB", "Host:a", "Team:A"}
	filters := []struct{ include, exclude []string }{
		{},
		{exclude: []string{"env:prod"}},
		{exclude: []string{"team:*", "stream:std*"}},
		{include: []string{"team:a"}, exclude: []string{"team:*"}},
		{exclude: []string{"sourcecategory:cat", "region:*"}},
		{exclude: []string{"service:*", "host:*", "team:a"}},
	}
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
		rs := ruleSetOf(t, tagLists...)
		category := ""
		if rng.IntN(2) == 0 {
			category = "cat"
		}
		service := []string{"", "web", "api", "API"}[rng.IntN(4)]
		src := newServiceSource(pick(2), pick(2), category, service)
		filter := filters[rng.IntN(len(filters))]
		setFilter(t, src.source, filter.include, filter.exclude)
		for range 20 {
			src.msgService = []string{"", "web", "api", "db", "Web"}[rng.IntN(5)]
			dynamic, msgTags := pick(3), pick(2)
			msg := src.msg(dynamic, msgTags)
			_, _, _, ok := msg.Origin.TagSnapshot()
			require.True(t, ok)
			want := transportMatch(rs, msg.Origin)
			require.Same(t, want, rs.match(msg.Origin),
				"round %d quotas %v filter %v tags %v", round, tagLists, filter, msg.Origin.Tags())
			require.Same(t, want, rs.match(src.fallbackMsg(dynamic, msgTags).Origin),
				"fallback round %d quotas %v filter %v tags %v", round, tagLists, filter, msg.Origin.Tags())
		}
	}
}

func TestPlanEarlierPartialBeatsLaterFull(t *testing.T) {
	rs := ruleSetOf(t, []string{"env:prod", "stream:stderr"}, []string{"env:prod"})
	src := newSnapshotSource(nil, []string{"env:prod"}, "")
	assert.Same(t, rs.quotas[0], rs.match(src.msg(nil, []string{"stream:stderr"}).Origin))
	assert.Same(t, rs.quotas[1], rs.match(src.msg(nil, []string{"stream:stdout"}).Origin))
}

func TestFilteredSnapshotTagSkipsQuota(t *testing.T) {
	rs := ruleSetOf(t, []string{"env:prod", "team:a"}, []string{"env:prod"})
	src := newSnapshotSource(nil, []string{"env:prod", "team:a"}, "")
	setFilter(t, src.source, nil, []string{"team:*"})
	assert.Same(t, rs.quotas[1], rs.match(src.msg(nil, nil).Origin))
	assert.Same(t, rs.quotas[1], rs.match(src.fallbackMsg(nil, nil).Origin))

	dropped := newSnapshotSource(nil, []string{"env:prod", "team:a"}, "")
	setFilter(t, dropped.source, nil, []string{"Env:*"})
	assert.Nil(t, rs.match(dropped.msg(nil, nil).Origin))
	assert.Nil(t, rs.match(dropped.fallbackMsg(nil, nil).Origin))
}

func TestPayloadAttributeTagsStillMatch(t *testing.T) {
	rs := ruleSetOf(t, []string{"service:web", "team:a"}, []string{"Host:a"})
	src := newSnapshotSource(nil, []string{"service:web", "team:a", "Host:a"}, "")
	setFilter(t, src.source, nil, []string{"Service:*", "host:*"})
	assert.Same(t, rs.quotas[0], rs.match(src.msg(nil, nil).Origin))
	assert.Same(t, rs.quotas[0], rs.match(src.fallbackMsg(nil, nil).Origin))

	partial := newSnapshotSource(nil, []string{"service:web", "team:a", "Host:a"}, "")
	setFilter(t, partial.source, nil, []string{"service:*", "team:*"})
	assert.Same(t, rs.quotas[1], rs.match(partial.msg(nil, nil).Origin))
	assert.Same(t, rs.quotas[1], rs.match(partial.fallbackMsg(nil, nil).Origin))
}

func TestTopLevelServiceMatches(t *testing.T) {
	rs := ruleSetOf(t, []string{"service:web", "env:prod"}, []string{"service:api"})
	src := newServiceSource(nil, []string{"env:prod"}, "", "web")
	setFilter(t, src.source, nil, []string{"service:*"})
	assert.Same(t, rs.quotas[0], rs.match(src.msg(nil, nil).Origin))
	assert.Same(t, rs.quotas[0], rs.match(src.fallbackMsg(nil, nil).Origin))

	other := newServiceSource(nil, []string{"env:prod"}, "", "db")
	assert.Nil(t, rs.match(other.msg(nil, nil).Origin))
	assert.Nil(t, rs.match(other.fallbackMsg(nil, nil).Origin))

	journal := message.NewOrigin(sources.NewLogSource("", &config.LogsConfig{}))
	journal.SetService("api")
	assert.Same(t, rs.quotas[1], rs.match(journal))
}

func TestMessageServiceMatchesThroughSnapshot(t *testing.T) {
	rs := ruleSetOf(t, []string{"env:prod", "service:web"}, []string{"service:api", "team:a"}, []string{"service:db"})
	src := newSnapshotSource(nil, []string{"env:prod"}, "")
	snap := src.msg(nil, nil)
	shared, _, _, _ := snap.Origin.TagSnapshot()

	for service, want := range map[string]*quota{"web": rs.quotas[0], "api": nil, "db": rs.quotas[2], "": nil} {
		src.msgService = service
		msg := src.msg(nil, nil)
		got, _, _, ok := msg.Origin.TagSnapshot()
		require.True(t, ok)
		require.Same(t, shared, got, "a per-message service must not replace the snapshot")
		assert.Same(t, want, rs.match(msg.Origin), service)
	}
	src.msgService = "api"
	assert.Same(t, rs.quotas[1], rs.match(src.msg(nil, []string{"team:a"}).Origin))
}

func TestFilteredMessageTagSkipsQuota(t *testing.T) {
	rs := ruleSetOf(t, []string{"env:prod", "stream:stderr"}, []string{"env:prod"})
	src := newSnapshotSource(nil, []string{"env:prod"}, "")
	setFilter(t, src.source, nil, []string{"stream:*"})
	assert.Same(t, rs.quotas[1], rs.match(src.msg(nil, []string{"stream:stderr"}).Origin))
	assert.Same(t, rs.quotas[1], rs.match(src.fallbackMsg(nil, []string{"stream:stderr"}).Origin))
}

func TestPlanCachedPerFilter(t *testing.T) {
	rs := ruleSetOf(t, []string{"team:a"})
	src := newSnapshotSource(nil, []string{"team:a"}, "")
	snap, _, _, _ := src.msg(nil, nil).Origin.TagSnapshot()
	f, _ := tagfilter.Compile(nil, []string{"team:*"})
	scoped := tagfilter.NewScoped(f, nil)

	unfiltered := rs.planFor(snap, nil)
	filtered := rs.planFor(snap, scoped)
	assert.NotSame(t, unfiltered, filtered)
	assert.Equal(t, 0, unfiltered.match(nil, ""))
	assert.Equal(t, 1, filtered.match(nil, ""))
	assert.Same(t, filtered, rs.planFor(snap, scoped))
}

func TestPlanReusedUntilSnapshotOrRuleSetChanges(t *testing.T) {
	rs := ruleSetOf(t, []string{"team:a"})
	src := newSnapshotSource(nil, nil, "")
	m1 := src.msg([]string{"team:a"}, nil)
	m2 := src.msg([]string{"team:a"}, nil)
	s1, _, _, _ := m1.Origin.TagSnapshot()
	s2, _, _, _ := m2.Origin.TagSnapshot()
	require.Same(t, s1, s2)
	assert.Same(t, rs.planFor(s1, nil), rs.planFor(s2, nil))

	m3 := src.msg([]string{"team:b"}, nil)
	assert.Nil(t, rs.match(m3.Origin))
	assert.Same(t, rs.quotas[0], rs.match(m1.Origin), "queued message keeps its own snapshot")

	rs2 := ruleSetOf(t, []string{"team:b"})
	assert.Same(t, rs2.quotas[0], rs2.match(m3.Origin))
	assert.Nil(t, rs2.match(m1.Origin))
}

func TestSnapshotIgnoredAfterConfigTagsChange(t *testing.T) {
	rs := ruleSetOf(t, []string{"env:prod"})
	src := newSnapshotSource([]string{"env:prod"}, nil, "")
	msg := src.msg(nil, nil)
	require.Same(t, rs.quotas[0], rs.match(msg.Origin))

	src.source.Config.Tags = []string{"env:dev"}
	_, _, _, ok := msg.Origin.TagSnapshot()
	assert.False(t, ok)
	assert.Nil(t, rs.match(msg.Origin))
}

func TestCheckWithSnapshotExpiry(t *testing.T) {
	now := t0
	e := newTestEnforcer(&now)
	e.Update(map[string][]byte{
		"a": snap(t0.Add(time.Minute), []string{"app:x", "env:prod"}),
		"b": snap(t0.Add(time.Hour), []string{"env:prod"}),
	})
	src := newSnapshotSource(nil, []string{"env:prod", "app:x"}, "")
	require.True(t, e.Check(src.msg(nil, nil)))
	now = t0.Add(2 * time.Minute)
	e.tick()
	require.True(t, e.Check(src.msg(nil, nil)))
	now = t0.Add(2 * time.Hour)
	e.tick()
	assert.False(t, e.Check(src.msg(nil, nil)))

	_, configs := e.Status()
	assert.EqualValues(t, 1, configs[0].Quotas[0].DroppedEvents)
	assert.EqualValues(t, 1, configs[1].Quotas[0].DroppedEvents)
}

func BenchmarkCheckSnapshot(b *testing.B) {
	for _, nTags := range []int{8, 47} {
		static := make([]string, nTags-1)
		for i := range static {
			static[i] = "key" + strconv.Itoa(i) + ":value"
		}
		src := newSnapshotSource(nil, static, "")
		msgTags := []string{"stream:stdout"}
		linear := message.NewMessageWithSource([]byte("hello"), "", sources.NewLogSource("", &config.LogsConfig{}), 0)
		linear.Origin.SetTags(slices.Concat(static, msgTags))
		snapshotted := src.msg(nil, msgTags)

		filteredSrc := newSnapshotSource(nil, static, "")
		setFilter(b, filteredSrc.source, nil, []string{"drop:*", "Other:*"})
		filteredLinear := filteredSrc.fallbackMsg(nil, msgTags)
		filteredSnapshot := filteredSrc.msg(nil, msgTags)

		for _, n := range []int{0, 1, 5, 10, 100, 500} {
			e := NewEnforcer()
			e.Update(sharedPrefixQuotas(n, static[0], time.Now().Add(time.Hour)))
			b.Run(fmt.Sprintf("tags=%d/quotas=%d/linear", nTags, n), func(b *testing.B) {
				for b.Loop() {
					e.Check(linear)
				}
			})
			b.Run(fmt.Sprintf("tags=%d/quotas=%d/snapshot", nTags, n), func(b *testing.B) {
				for b.Loop() {
					e.Check(snapshotted)
				}
			})
			b.Run(fmt.Sprintf("tags=%d/quotas=%d/snapshot+rebuild", nTags, n), func(b *testing.B) {
				i := 0
				for b.Loop() {
					i++
					e.Check(src.msg([]string{strconv.Itoa(i)}, msgTags))
				}
			})
			b.Run(fmt.Sprintf("tags=%d/quotas=%d/linear+filter", nTags, n), func(b *testing.B) {
				for b.Loop() {
					e.Check(filteredLinear)
				}
			})
			b.Run(fmt.Sprintf("tags=%d/quotas=%d/snapshot+filter", nTags, n), func(b *testing.B) {
				for b.Loop() {
					e.Check(filteredSnapshot)
				}
			})
			b.Run(fmt.Sprintf("tags=%d/quotas=%d/snapshot+filter+rebuild", nTags, n), func(b *testing.B) {
				i := 0
				for b.Loop() {
					i++
					e.Check(filteredSrc.msg([]string{strconv.Itoa(i)}, msgTags))
				}
			})
		}
	}
}

func BenchmarkCheckMessageService(b *testing.B) {
	static := []string{"env:prod", "team:a", "region:us"}
	for _, n := range []int{10, 100, 500} {
		configs := make(map[string][]byte, n)
		for i := range n {
			configs[strconv.Itoa(i)] = snap(time.Now().Add(time.Hour), []string{"service:svc" + strconv.Itoa(i)})
		}
		e := NewEnforcer()
		e.Update(configs)
		for _, service := range []string{"svc" + strconv.Itoa(n-1), "other"} {
			src := newSnapshotSource(nil, static, "")
			src.msgService = service
			snapshotted, linear := src.msg(nil, nil), src.fallbackMsg(nil, nil)
			b.Run(fmt.Sprintf("quotas=%d/service=%s/linear", n, service), func(b *testing.B) {
				for b.Loop() {
					e.Check(linear)
				}
			})
			b.Run(fmt.Sprintf("quotas=%d/service=%s/snapshot", n, service), func(b *testing.B) {
				for b.Loop() {
					e.Check(snapshotted)
				}
			})
		}
	}
}

// sharedPrefixQuotas returns n quotas that all require shared, then fail on a tag no message has.
func sharedPrefixQuotas(n int, shared string, expiresAt time.Time) map[string][]byte {
	configs := make(map[string][]byte, n)
	for i := range n {
		configs[strconv.Itoa(i)] = snap(expiresAt, []string{shared, "team:t" + strconv.Itoa(i)})
	}
	return configs
}
