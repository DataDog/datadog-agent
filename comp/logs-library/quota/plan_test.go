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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type snapshotSource struct {
	source      *sources.LogSource
	static      []string
	snapshotter message.TagSnapshotter
}

func newSnapshotSource(configTags, static []string, category string) *snapshotSource {
	return &snapshotSource{
		source: sources.NewLogSource("", &config.LogsConfig{Tags: configTags, SourceCategory: category}),
		static: static,
	}
}

func (s *snapshotSource) msg(dynamic, msgTags []string) *message.Message {
	origin := message.NewOrigin(s.source)
	tags := slices.Concat(s.static, dynamic, msgTags)
	origin.SetTagsWithSnapshot(tags, s.snapshotter.Get(s.source, s.static, dynamic), msgTags)
	return message.NewMessage([]byte("hello"), origin, "", 0)
}

func linearMatch(rs *ruleSet, o *message.Origin) *quota {
	for _, q := range rs.quotas {
		if q.matches(o) {
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
	vocab := []string{"env:prod", "env:dev", "team:a", "team:b", "stream:stderr", "stream:stdout", "app:x", "sourcecategory:cat", "sourcecategory:other", "region:us"}
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
		src := newSnapshotSource(pick(2), pick(2), category)
		for range 20 {
			msg := src.msg(pick(3), pick(2))
			_, _, ok := msg.Origin.TagSnapshot()
			require.True(t, ok)
			require.Same(t, linearMatch(rs, msg.Origin), rs.match(msg.Origin),
				"round %d quotas %v tags %v", round, tagLists, msg.Origin.Tags())
		}
	}
}

func TestPlanEarlierPartialBeatsLaterFull(t *testing.T) {
	rs := ruleSetOf(t, []string{"env:prod", "stream:stderr"}, []string{"env:prod"})
	src := newSnapshotSource(nil, []string{"env:prod"}, "")
	assert.Same(t, rs.quotas[0], rs.match(src.msg(nil, []string{"stream:stderr"}).Origin))
	assert.Same(t, rs.quotas[1], rs.match(src.msg(nil, []string{"stream:stdout"}).Origin))
}

func TestPlanReusedUntilSnapshotOrRuleSetChanges(t *testing.T) {
	rs := ruleSetOf(t, []string{"team:a"})
	src := newSnapshotSource(nil, nil, "")
	m1 := src.msg([]string{"team:a"}, nil)
	m2 := src.msg([]string{"team:a"}, nil)
	s1, _, _ := m1.Origin.TagSnapshot()
	s2, _, _ := m2.Origin.TagSnapshot()
	require.Same(t, s1, s2)
	assert.Same(t, rs.planFor(s1), rs.planFor(s2))

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
	_, _, ok := msg.Origin.TagSnapshot()
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
	require.True(t, e.Check(src.msg(nil, nil)))
	now = t0.Add(2 * time.Hour)
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
