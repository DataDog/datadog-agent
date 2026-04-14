// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
package clustering

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering/merging"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Cluster represents a cluster with a group of TokenLists that have identical signatures.
// A cluster may contain multiple patterns if token lists with the same signature cannot be merged since structural Fidelity is Valuable.
// Examples:
// "Status: OK"     → HTTP response format
// "Status; OK"     → CSV-like format
// "Status OK"      → Plain text format
// These are different log formats, even if semantically similar → we need to keep them separate.
type Cluster struct {
	Signature token.Signature
	Patterns  []*Pattern // Multiple patterns per cluster

	// Timestamp tracking for the cluster itself
	CreatedAt time.Time // When cluster was first created
	UpdatedAt time.Time // When cluster was last modified (any pattern changed)

	// firstWordProtection is inherited from ClusterManager at cluster creation time.
	// It can be automatically set to false when firstWordUniqueValues exceeds
	// firstWordMaxCardinality (adaptive maxChild behavior).
	firstWordProtection bool

	// firstWordPos is the position of the first TokenWord in this cluster's token sequence.
	// -1 if no TokenWord exists. Computed lazily on first pattern creation.
	firstWordPos int

	// firstWordUniqueValues tracks unique concrete values seen at firstWordPos.
	// Once len > firstWordMaxCardinality, firstWordProtection is auto-disabled.
	// Nilled after the threshold is reached to free memory.
	firstWordUniqueValues map[string]struct{}

	// firstWordMaxCardinality is the threshold for adaptive protection.
	// 0 means no adaptive override (always use firstWordProtection as-is).
	firstWordMaxCardinality int

	// lastMatchedPattern is a one-entry cache pointing to the most recently matched
	// pattern. In steady-state (post-convergence) the same 1-2 patterns absorb nearly
	// all traffic, so this makes the common case O(1) instead of O(len(Patterns)).
	// Nil initially and after the cached pattern is evicted.
	lastMatchedPattern *Pattern

	// saturatedThreshold is the number of consecutive identical merges (TryMerge returning
	// tl1 unchanged) before a pattern is marked saturated. Once saturated, the
	// CanMergeTokenLists pre-check is skipped for that pattern — single-pass TryMerge only.
	// 0 disables saturation scoring entirely.
	saturatedThreshold int

	// maxPatternsPerCluster caps len(Patterns). When at cap, new unmatched messages are
	// force-widened into the closest existing pattern instead of creating a new one.
	// 0 = unlimited (backward compatible).
	maxPatternsPerCluster int

	// scanBudget limits CanMerge iterations in the full-scan loop per message.
	// Move-to-front on hit keeps recently matched patterns within future budgets.
	// 0 = unlimited (backward compatible).
	scanBudget int
}

// NewCluster creates a new cluster.
func NewCluster(signature token.Signature, firstWordProtection bool, firstWordMaxCardinality int, saturatedThreshold int, maxPatternsPerCluster int, scanBudget int) *Cluster {
	now := time.Now()
	return &Cluster{
		Signature:               signature,
		Patterns:                nil, // Will be generated when needed
		CreatedAt:               now,
		UpdatedAt:               now,
		firstWordProtection:     firstWordProtection,
		firstWordPos:            -1, // set lazily on first pattern
		firstWordMaxCardinality: firstWordMaxCardinality,
		saturatedThreshold:      saturatedThreshold,
		maxPatternsPerCluster:   maxPatternsPerCluster,
		scanBudget:              scanBudget,
	}
}

// =============================================================================
// Core Clustering Logic
// =============================================================================

// AddTokenListToPatterns adds a TokenList to the appropriate pattern in the cluster.
// If no matching pattern exists, creates a new one.
// Returns the matched/new pattern plus the old wildcard count and estimated bytes
// of the matched pattern (both zero for new patterns), so the caller can compute deltas.
func (c *Cluster) AddTokenListToPatterns(tokenList *token.TokenList, cm *ClusterManager) (*Pattern, int, int64) {
	if len(c.Patterns) == 0 {
		// Compute and cache the first-word position for this cluster's token structure.
		c.firstWordPos = -1
		for i, tok := range tokenList.Tokens {
			if tok.Type == token.TokenWord {
				c.firstWordPos = i
				break
			}
		}
		patternID := cm.generatePatternID()
		pattern := newPattern(tokenList, patternID)
		c.Patterns = []*Pattern{pattern}
		c.lastMatchedPattern = pattern
		c.UpdatedAt = time.Now()
		return pattern, 0, 0
	}

	// Adaptive first-word protection: track unique concrete values at the first-word
	// position. When the count exceeds firstWordMaxCardinality, the position is
	// clearly a high-cardinality variable (e.g., username, transaction ID) rather than
	// a semantic discriminator — auto-disable protection for this cluster.
	effectiveProtection := c.firstWordProtection
	if effectiveProtection && c.firstWordMaxCardinality > 0 && c.firstWordPos >= 0 &&
		c.firstWordPos < len(tokenList.Tokens) {
		tok := &tokenList.Tokens[c.firstWordPos]
		if tok.Wildcard != token.IsWildcard && tok.Type == token.TokenWord {
			if c.firstWordUniqueValues == nil {
				c.firstWordUniqueValues = make(map[string]struct{})
			}
			c.firstWordUniqueValues[tok.Value] = struct{}{}
			if len(c.firstWordUniqueValues) > c.firstWordMaxCardinality {
				c.firstWordProtection = false // permanently disable for this cluster
				c.firstWordUniqueValues = nil // free tracking memory
				effectiveProtection = false
			}
		}
	}

	// Hot-pattern cache: try the last matched pattern first.
	// For saturated patterns (consecutiveIdenticalMerges >= threshold), skip the
	// CanMergeTokenLists pre-check (O(tokens)) and call TryMerge directly (single pass).
	// For non-saturated patterns, keep the existing two-pass (CanMerge + TryMerge).
	if c.lastMatchedPattern != nil && c.lastMatchedPattern.Template != nil {
		lmp := c.lastMatchedPattern
		oldWildcardCount := lmp.GetWildcardCount()
		oldBytes := lmp.EstimatedBytes()
		if lmp.saturated {
			// Saturated fast path: single O(tokens) TryMerge call, no CanMerge pre-check.
			if c.tryMergeIntoPattern(lmp, tokenList) {
				return lmp, oldWildcardCount, oldBytes
			}
			// Unexpected miss: desaturate and fall through to full scan.
			lmp.saturated = false
			lmp.consecutiveIdenticalMerges = 0
		} else {
			if merging.CanMergeTokenLists(lmp.Template, tokenList, effectiveProtection) {
				if c.tryMergeIntoPattern(lmp, tokenList) {
					return lmp, oldWildcardCount, oldBytes
				}
			}
		}
	}

	// Single pass: CanMerge against Template (not Sample), then TryMerge to apply.
	// Using Template instead of Sample eliminates the redundant CanMerge-against-Sample
	// pre-scan that ClusterManager.Add previously did. Template is at least as permissive
	// as Sample for CanMerge (one-sided first-word protection).
	// We cannot use TryMerge alone because it applies symmetric first-word protection
	// which is stricter and would reject valid merges.
	//
	// scanBudget limits how many patterns we CanMerge-check per message. Move-to-front
	// on hit keeps recently matched patterns within future budgets.
	scanned := 0
	for i, p := range c.Patterns {
		if p == c.lastMatchedPattern {
			// Already tried above; skip to avoid double-checking.
			continue
		}
		if p.Template == nil {
			continue
		}
		if c.scanBudget > 0 && scanned >= c.scanBudget {
			break // budget exhausted — fall through to new-pattern / force-widen
		}
		scanned++
		if !merging.CanMergeTokenLists(p.Template, tokenList, effectiveProtection) {
			continue
		}
		oldWildcardCount := p.GetWildcardCount()
		oldBytes := p.EstimatedBytes()
		if c.tryMergeIntoPattern(p, tokenList) {
			// Move-to-front: swap toward index 1 so future scan budgets cover it.
			// Index 0 is often the oldest pattern; lastMatchedPattern covers the #1 hot slot.
			if i > 1 {
				c.Patterns[i], c.Patterns[1] = c.Patterns[1], c.Patterns[i]
			}
			c.lastMatchedPattern = p
			return p, oldWildcardCount, oldBytes
		}
	}

	// At cap: force-widen into closest match instead of creating a new pattern.
	if c.maxPatternsPerCluster > 0 && len(c.Patterns) >= c.maxPatternsPerCluster {
		if best := c.findClosestPattern(tokenList); best != nil {
			oldWildcardCount := best.GetWildcardCount()
			oldBytes := best.EstimatedBytes()
			if widened := merging.ForceWiden(best.Template, tokenList); widened != nil {
				c.applyWidenedTemplate(best, widened)
				c.lastMatchedPattern = best
				return best, oldWildcardCount, oldBytes
			}
		}
		// Safety valve: ForceWiden only fails on length mismatch. All patterns in a cluster
		// share the same signature/token-count, so this should not happen in practice.
		// Fall through to newPattern as a defensive fallback.
	}

	patternID := cm.generatePatternID()
	pattern := newPattern(tokenList, patternID)
	c.Patterns = append(c.Patterns, pattern)
	c.lastMatchedPattern = pattern // new pattern will likely be hit again
	c.UpdatedAt = time.Now()
	return pattern, 0, 0
}

// tryMergeIntoPattern attempts to merge tokenList into an existing pattern using
// TryMergeTokenLists (single-pass CanMerge+Merge). Returns true if merge succeeded.
func (c *Cluster) tryMergeIntoPattern(p *Pattern, tokenList *token.TokenList) bool {
	if p.Template == nil {
		return false
	}

	// Single-pass: check template compatibility and merge in one traversal.
	// TryMergeTokenLists is symmetric (protects first-word from either list),
	// so a single call suffices.
	merged := merging.TryMergeTokenLists(p.Template, tokenList, c.firstWordProtection)
	if merged == nil {
		return false
	}

	now := time.Now()
	p.LogCount++
	p.LastAccessAt = now
	p.UpdatedAt = now
	c.UpdatedAt = now

	if merged == p.Template {
		// Zero-alloc identical path: TryMerge returned the template pointer unchanged.
		// The incoming log was structurally identical — no wildcards added.
		// Track consecutive identical merges for saturation scoring.
		if c.saturatedThreshold > 0 {
			p.consecutiveIdenticalMerges++
			if p.consecutiveIdenticalMerges >= c.saturatedThreshold {
				p.saturated = true
			}
		}
		// Template and Positions are unchanged — skip the rebuild loop below.
		return true
	}

	// Template structurally changed (new wildcards added): reset saturation.
	p.consecutiveIdenticalMerges = 0
	p.saturated = false

	// Apply the merged template
	p.Template = merged
	p.Positions = p.Positions[:0]

	n := len(merged.Tokens)
	for i := 0; i < n; i++ {
		tok := &merged.Tokens[i]
		if tok.Wildcard == token.IsWildcard {
			p.Positions = append(p.Positions, i)

			if tok.Type == token.TokenAbsolutePath && p.Sample != nil && i < len(p.Sample.Tokens) {
				tok.Value = getPathPattern(p.Sample.Tokens[i].Value)
			}
		}
	}

	return true
}

// findClosestPattern returns the pattern with the most token positions identical
// to tokenList (fewest positions that would need wildcarding under ForceWiden).
func (c *Cluster) findClosestPattern(tokenList *token.TokenList) *Pattern {
	var best *Pattern
	bestScore := -1
	for _, p := range c.Patterns {
		if p.Template == nil {
			continue
		}
		score := countIdentical(p.Template, tokenList)
		if score > bestScore {
			bestScore = score
			best = p
		}
	}
	return best
}

// countIdentical returns the number of token positions where both lists are identical.
func countIdentical(tl1, tl2 *token.TokenList) int {
	n := len(tl1.Tokens)
	if len(tl2.Tokens) < n {
		n = len(tl2.Tokens)
	}
	count := 0
	for i := 0; i < n; i++ {
		if tl1.Tokens[i].Compare(&tl2.Tokens[i]) == token.Identical {
			count++
		}
	}
	return count
}

// applyWidenedTemplate updates a pattern with a force-widened template, rebuilds
// Positions, resets saturation state, and updates timestamps.
// If widened == p.Template (pointer-same, all-identical), only LogCount is bumped.
func (c *Cluster) applyWidenedTemplate(p *Pattern, widened *token.TokenList) {
	p.LogCount++
	p.LastAccessAt = time.Now()
	if widened == p.Template {
		return // all-identical, nothing changed structurally
	}
	p.Template = widened
	p.UpdatedAt = time.Now()
	c.UpdatedAt = time.Now()
	// Rebuild wildcard positions
	p.Positions = p.Positions[:0]
	for i, tok := range widened.Tokens {
		if tok.Wildcard == token.IsWildcard {
			p.Positions = append(p.Positions, i)
		}
	}
	// Reset saturation — template changed structurally
	p.consecutiveIdenticalMerges = 0
	p.saturated = false
}

// getPathPattern converts a path to hierarchical wildcard pattern
func getPathPattern(path string) string {
	if path == "/" {
		return "/"
	}

	// Remove leading/trailing slashes and split
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "/"
	}

	parts := strings.Split(trimmed, "/")
	result := ""
	for i := 0; i < len(parts); i++ {
		result += "/*"
	}

	return result
}
