// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logsignature

import (
	"container/list"
	"slices"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/logs/tokenizer"
)

// Group separates independent log populations before matching. Full metric tags
// remain the caller's responsibility; this key must not retain borrowed buffers.
type Group struct {
	Source, Service, Env, Host string
}

// CacheConfig bounds both retained state and the work of a pattern lookup.
// Callers must supply positive limits and a threshold in (0, 1].
type CacheConfig struct {
	MaxEvalBytes, MaxPatterns, MaxPatternsPerGroup, MaxGroups int
	IdleTTLSeconds                                            int64
	MatchThreshold                                            float64
}

// Pattern is a fixed, owned representative of one fuzzy-matched population.
// SeriesName is unique for the cache's lifetime, including after eviction. This
// prevents cleanup of one group from deleting another group's metric series.
type Pattern struct {
	SeriesName string
	Signature  string
	tokens     []tokenizer.Token
	count      uint64
	lastSeen   int64
	index      int
	group      *patternGroup
	globalLRU  *list.Element
	groupLRU   *list.Element
}

type patternGroup struct {
	key      Group
	patterns []*Pattern // Descending frequency; ties preserve matching order.
	lru      list.List
	global   *list.Element
}

// Cache owns one reusable tokenizer and bounded pattern/group indexes. It is
// single-writer, like the Observer engine. Tokenization and matching semantics
// are supplied entirely by pkg/logs/tokenizer.
type Cache struct {
	config    CacheConfig
	tokenizer *tokenizer.Tokenizer
	groups    map[Group]*patternGroup
	groupLRU  list.List
	patterns  list.List
	nextID    uint64
	watermark int64
}

// NewCache constructs a cache with validated, positive bounds.
func NewCache(config CacheConfig) *Cache {
	return &Cache{config: config, tokenizer: tokenizer.NewTokenizer(config.MaxEvalBytes), groups: make(map[Group]*patternGroup)}
}

// Len returns the number of retained representatives across all groups.
func (c *Cache) Len() int { return c.patterns.Len() }

// Reset releases representatives, group identities, and tokenizer scratch.
// The caller resets storage at the same time, so series IDs may restart.
func (c *Cache) Reset() {
	*c = *NewCache(c.config)
}

// Match returns a representative and the series names whose state must be
// removed before emitting the new observation. Empty logs do not admit groups.
// Activity uses a monotonic data-time watermark: late logs cannot move an LRU
// timestamp backwards or break expiry ordering.
func (c *Cache) Match(group Group, content string, timestamp int64) (*Pattern, []string) {
	if timestamp > c.watermark {
		c.watermark = timestamp
	}
	var evicted []string
	cutoff := c.watermark - c.config.IdleTTLSeconds
	for front := c.patterns.Front(); front != nil; front = c.patterns.Front() {
		p := front.Value.(*Pattern)
		if p.lastSeen >= cutoff {
			break
		}
		c.remove(p, &evicted)
	}
	tokens := c.tokenizer.TokenizeBorrowed([]byte(content)).Borrow()
	if len(tokens) == 0 {
		return nil, evicted
	}
	g := c.groups[group]
	if g != nil {
		for _, p := range g.patterns {
			if !tokenizer.IsMatch(p.tokens, tokens, c.config.MatchThreshold) {
				continue
			}
			p.count++
			p.lastSeen = c.watermark
			c.patterns.MoveToBack(p.globalLRU)
			g.lru.MoveToBack(p.groupLRU)
			c.groupLRU.MoveToBack(g.global)
			for p.index > 0 && p.count > g.patterns[p.index-1].count {
				i := p.index
				g.patterns[i], g.patterns[i-1] = g.patterns[i-1], p
				g.patterns[i].index = i
				p.index--
			}
			return p, evicted
		}
		if len(g.patterns) >= c.config.MaxPatternsPerGroup {
			c.remove(g.lru.Front().Value.(*Pattern), &evicted)
		}
	}
	for c.patterns.Len() >= c.config.MaxPatterns {
		c.remove(c.patterns.Front().Value.(*Pattern), &evicted)
	}
	// Capacity eviction may have removed the last pattern of this group.
	g = c.groups[group]
	if g == nil {
		if len(c.groups) >= c.config.MaxGroups {
			victim := c.groupLRU.Front().Value.(*patternGroup)
			for victim.lru.Len() > 0 {
				c.remove(victim.lru.Front().Value.(*Pattern), &evicted)
			}
		}
		g = &patternGroup{key: group}
		g.global = c.groupLRU.PushBack(g)
		c.groups[group] = g
	}
	c.nextID++
	p := &Pattern{
		SeriesName: "log.pattern." + strconv.FormatUint(c.nextID, 16) + ".count",
		Signature:  tokenizer.TokensToString(tokens),
		tokens:     slices.Clone(tokens), // Borrowed tokens expire at the next call.
		count:      1,
		lastSeen:   c.watermark,
		index:      len(g.patterns),
		group:      g,
	}
	p.globalLRU = c.patterns.PushBack(p)
	p.groupLRU = g.lru.PushBack(p)
	g.patterns = append(g.patterns, p)
	c.groupLRU.MoveToBack(g.global)
	return p, evicted
}

func (c *Cache) remove(p *Pattern, evicted *[]string) {
	g := p.group
	*evicted = append(*evicted, p.SeriesName)
	c.patterns.Remove(p.globalLRU)
	g.lru.Remove(p.groupLRU)
	copy(g.patterns[p.index:], g.patterns[p.index+1:])
	g.patterns[len(g.patterns)-1] = nil
	g.patterns = g.patterns[:len(g.patterns)-1]
	for i := p.index; i < len(g.patterns); i++ {
		g.patterns[i].index = i
	}
	if len(g.patterns) == 0 {
		delete(c.groups, g.key)
		c.groupLRU.Remove(g.global)
	}
}
