// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package patterns provides log tokenization and clustering utilities.
package patterns

import (
	"fmt"
	"strings"
)

// Cluster represents a group of similar log messages.
type Cluster struct {
	// TODO(celian): Use a map to efficiently get the cluster by ID
	ID        int64
	Signature string
	Pattern   []Token
	Count     int
	Tags      map[string]string
	Samples   []string
}

// "Shallow" copy of the cluster
type ClusterInfo struct {
	ID            int64
	Signature     string
	PatternString string
	Count         int
	FirstSample   string
}

// PatternString returns the human-readable pattern for this cluster.
func (c *Cluster) PatternString() string {
	var b strings.Builder
	for _, t := range c.Pattern {
		b.WriteString(t.PatternString())
	}
	return b.String()
}

func (c *Cluster) ToClusterInfo() ClusterInfo {
	return ClusterInfo{
		ID:            c.ID,
		Signature:     c.Signature,
		PatternString: c.PatternString(),
		Count:         c.Count,
		FirstSample:   c.Samples[0],
	}
}

// SignatureClusterer groups logs by exact signature match.
type SignatureClusterer struct {
	clusters    map[string]*Cluster
	orderedKeys []string
	tokenizer   *Tokenizer
	IgnoreEmpty bool
	TextGetter  func(doc map[string]string) string
	TagGetters  map[string]func(doc map[string]string) string
	nextID      int64
}

func NewSignatureClusterer() *SignatureClusterer {
	return &SignatureClusterer{
		clusters:    make(map[string]*Cluster),
		tokenizer:   NewTokenizer(),
		IgnoreEmpty: true,
		TextGetter:  func(doc map[string]string) string { return doc["message"] },
		TagGetters:  map[string]func(doc map[string]string) string{},
	}
}

func (sc *SignatureClusterer) Process(message string) (*Cluster, bool) {
	if sc.IgnoreEmpty && strings.TrimSpace(message) == "" {
		return nil, false
	}

	tokens := sc.tokenizer.Tokenize(message)
	return sc.ProcessTokens(tokens, message)
}

func (sc *SignatureClusterer) ProcessTokens(tokens []Token, message string) (*Cluster, bool) {
	sig := TokenListSignature(tokens)

	if c, ok := sc.clusters[sig]; ok {
		c.Count++
		return c, true
	}

	c := &Cluster{
		Signature: sig,
		Pattern:   tokens,
		Count:     1,
		Samples:   []string{message},
		ID:        sc.nextID,
	}
	sc.clusters[sig] = c
	sc.orderedKeys = append(sc.orderedKeys, sig)
	sc.nextID++

	return c, true
}

func (sc *SignatureClusterer) ProcessDoc(doc map[string]string) (*Cluster, bool) {
	message := sc.TextGetter(doc)
	result, ok := sc.Process(message)
	if ok && result.Count == 1 && len(sc.TagGetters) > 0 {
		if result.Tags == nil {
			result.Tags = make(map[string]string)
		}
		for tagName, getter := range sc.TagGetters {
			result.Tags[tagName] = getter(doc)
		}
	}
	return result, ok
}

func (sc *SignatureClusterer) GetClusters() []*Cluster {
	result := make([]*Cluster, 0, len(sc.orderedKeys))
	for _, k := range sc.orderedKeys {
		result = append(result, sc.clusters[k])
	}
	return result
}

// PatternClusterer groups logs by merging similar token patterns.
// It uses signature-based initial grouping then merges patterns within each group.
type PatternClusterer struct {
	clustersBySignature map[string][]*Cluster
	allClusters         []*Cluster
	tokenizer           *Tokenizer
	IgnoreEmpty         bool
	nextID              int64
	// minTokenMatchRatio is the effective minimum fraction of token positions that must
	// match by value for an incoming line to merge into an existing cluster. Set at
	// construction from rawMinTokenMatchRatio via effectiveMinTokenMatchRatio.
	minTokenMatchRatio float64
}

func NewPatternClusterer() *PatternClusterer {
	return NewPatternClustererWithTokenizer(NewTokenizer(), 0)
}

// NewPatternClustererWithTokenizer creates a PatternClusterer that uses the given tokenizer.
// If t is nil, a default Tokenizer is used. rawMinTokenMatchRatio is normalized once at
// construction (≤0 → 0.5, >1 → 1); pass 0 for the library default.
func NewPatternClustererWithTokenizer(t *Tokenizer, rawMinTokenMatchRatio float64) *PatternClusterer {
	if t == nil {
		t = NewTokenizer()
	}
	return &PatternClusterer{
		clustersBySignature: make(map[string][]*Cluster),
		tokenizer:           t,
		IgnoreEmpty:         true,
		minTokenMatchRatio:  effectiveMinTokenMatchRatio(rawMinTokenMatchRatio),
	}
}

func (pc *PatternClusterer) NumClusters() int {
	return len(pc.allClusters)
}

func (pc *PatternClusterer) Process(message string) (*Cluster, bool) {
	if pc.IgnoreEmpty && strings.TrimSpace(message) == "" {
		return nil, false
	}

	tokens := pc.tokenizer.Tokenize(message)

	return pc.ProcessTokens(tokens, message)
}

func (pc *PatternClusterer) ProcessTokens(tokens []Token, message string) (*Cluster, bool) {
	sig := TokenListSignature(tokens)

	// Try within same signature group first
	clusters := pc.clustersBySignature[sig]
	for _, c := range clusters {
		if pc.canMergeTokenLists(c.Pattern, tokens) {
			mergeTokenLists(c.Pattern, tokens)
			c.Count++
			return c, true
		}
	}

	// Fallback: try other signature groups (handles minor structural differences
	// like path with/without trailing "?")
	for otherSig, otherClusters := range pc.clustersBySignature {
		if otherSig == sig {
			continue
		}
		for _, c := range otherClusters {
			if pc.canMergeTokenLists(c.Pattern, tokens) {
				mergeTokenLists(c.Pattern, tokens)
				c.Count++
				return c, true
			}
		}
	}

	c := &Cluster{
		Signature: sig,
		Pattern:   tokens,
		Count:     1,
		Samples:   []string{message},
		ID:        pc.nextID,
	}
	pc.clustersBySignature[sig] = append(pc.clustersBySignature[sig], c)
	pc.allClusters = append(pc.allClusters, c)
	pc.nextID++

	return c, true
}

func (pc *PatternClusterer) GetClusters() []*Cluster {
	return pc.allClusters
}

func (pc *PatternClusterer) GetCluster(id int64) (*Cluster, error) {
	// TODO: Optimize with map
	for _, c := range pc.allClusters {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("cluster %d not found", id)
}

func effectiveMinTokenMatchRatio(r float64) float64 {
	switch {
	case r <= 0:
		return 0.5
	case r > 1:
		return 1
	default:
		return r
	}
}

// canMergeTokenLists checks if two token lists can be merged.
// It requires that all token pairs be type-compatible AND that the fraction of
// positions with equal values is at least pc.minTokenMatchRatio (set at construction).
func (pc *PatternClusterer) canMergeTokenLists(pattern, incoming []Token) bool {
	return canMergeTokenListsWithRatio(pattern, incoming, pc.minTokenMatchRatio)
}

// canMergeTokenListsWithRatio is the merge predicate parameterized by an already-resolved
// minRatio. Used by tests and PatternClusterer.canMergeTokenLists.
func canMergeTokenListsWithRatio(pattern, incoming []Token, minRatio float64) bool {
	if len(pattern) != len(incoming) {
		return false
	}
	matching := 0
	for i := range pattern {
		if !canMergeTokens(pattern[i], incoming[i]) {
			return false
		}
		if pattern[i].Value == incoming[i].Value {
			matching++
		}
	}
	if len(pattern) == 0 {
		return true
	}
	return float64(matching) >= minRatio*float64(len(pattern))
}

func canMergeTokens(a, b Token) bool {
	if a.Type != b.Type {
		switch {
		case (a.Type == TypeNumericValue && b.Type == TypeHTTPStatusCode) ||
			(a.Type == TypeHTTPStatusCode && b.Type == TypeNumericValue):
			return true
		default:
			return false
		}
	}

	switch a.Type {
	case TypeSpecialCharacter:
		return a.Value == b.Value
	case TypeWhitespace:
		return true
	case TypeWord:
		if a.NeverWildcard && a.Value != b.Value {
			return false
		}
		return true
	case TypeNumericValue:
		return true
	case TypeDate, TypeLocalTime:
		return a.extra.DateFormat == b.extra.DateFormat
	case TypeIPv4Address:
		return true
	case TypeAbsolutePath, TypePathQueryFragment:
		return sameSegmentCount(a.extra.Segments, b.extra.Segments)
	case TypeURI:
		if a.extra.Scheme != b.extra.Scheme {
			return false
		}
		return true
	case TypeAuthority:
		return true
	case TypeEmailAddress:
		return true
	case TypeHTTPMethod:
		return true
	case TypeHTTPStatusCode:
		return true
	case TypeSeverity:
		return true
	case TypeHexDump:
		return a.extra.DispLen == b.extra.DispLen
	case TypeKVSequence:
		return a.extra.KVSep == b.extra.KVSep && a.extra.KVPairSep == b.extra.KVPairSep &&
			a.extra.KVQuote == b.extra.KVQuote && sameKeys(a.extra.KVKeys, b.extra.KVKeys)
	default:
		return a.Value == b.Value
	}
}

func sameSegmentCount(a, b []string) bool {
	return len(a) == len(b)
}

func sameKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// mergeTokenLists merges incoming tokens into the pattern, wildcarding differences.
func mergeTokenLists(pattern, incoming []Token) {
	for i := range pattern {
		if pattern[i].Value != incoming[i].Value {
			pattern[i].IsWild = true
		}
	}
}

// Classify returns the cluster matching the given message without modifying any state.
// It returns the matching cluster, or nil if no existing cluster matches.
func (pc *PatternClusterer) Classify(message string) *Cluster {
	if pc.IgnoreEmpty && strings.TrimSpace(message) == "" {
		return nil
	}
	tokens := pc.tokenizer.Tokenize(message)
	sig := TokenListSignature(tokens)

	if clusters, ok := pc.clustersBySignature[sig]; ok {
		for _, c := range clusters {
			if pc.canMergeTokenLists(c.Pattern, tokens) {
				return c
			}
		}
	}

	for otherSig, clusters := range pc.clustersBySignature {
		if otherSig == sig {
			continue
		}
		for _, c := range clusters {
			if pc.canMergeTokenLists(c.Pattern, tokens) {
				return c
			}
		}
	}

	return nil
}

// FormatCluster returns a formatted string describing a cluster.
func FormatCluster(c *Cluster) string {
	return fmt.Sprintf("sig=%s pattern=%s count=%d", c.Signature, c.PatternString(), c.Count)
}
