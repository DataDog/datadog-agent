// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"fmt"
	"sort"
	"strings"
)

// The compact format is meant to be fed to tools and LLMs with a size budget:
// ASCII indentation, one line per node, values inline, and collapsed repeats
// that carry no information of their own are dropped (but still counted).
//
// With a byte budget, the deepest subtrees are folded into their parent line,
// which keeps the number of assertions and the values found below it, until
// the output fits.

const (
	compactValueLen = 60
	// a folded line stands for a whole subtree: let it carry more values
	foldedValues = 24
)

// renderCompact renders nodes in the compact format; nodes in folded are
// printed on one line summarizing what is below them.
func renderCompact(nodes []*Node, folded map[*Node]bool) string {
	var b strings.Builder
	for _, n := range nodes {
		compactNode(&b, n, 0, folded)
		var s stats
		countStats(n, &s, false)
		fmt.Fprintf(&b, "= %d assertion(s), %d fatal, %d polling, %d conditional", s.assertions+s.repeated, s.fatal, s.polling, s.conditional)
		if s.implicit > 0 {
			fmt.Fprintf(&b, ", %d Must*", s.implicit)
		}
		if s.opaque > 0 {
			fmt.Fprintf(&b, ", %d not followed [?]", s.opaque)
		}
		if len(folded) > 0 {
			fmt.Fprintf(&b, " (%d node(s) folded to fit the size budget: their lines end with \"+N below\")", len(folded))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderCompactBudget renders the compact format within maxBytes when
// possible: it folds subtrees one at a time, deepest first (and, at the same
// depth, the largest first), so detail is removed evenly from the bottom and
// no more than needed. Folded lines keep the assertion count and the values
// found below them.
func renderCompactBudget(nodes []*Node, maxBytes int) string {
	folded := map[*Node]bool{}
	out := renderCompact(nodes, folded)
	if len(out) <= maxBytes {
		return out
	}
	type cand struct {
		n           *Node
		depth, size int
	}
	var cands []cand
	var collect func(n *Node, depth int)
	collect = func(n *Node, depth int) {
		if len(n.Children) == 0 {
			return
		}
		var b strings.Builder
		compactNode(&b, n, 0, nil)
		cands = append(cands, cand{n, depth, b.Len()})
		for _, c := range n.Children {
			collect(c, depth+1)
		}
	}
	for _, n := range nodes {
		for _, c := range n.Children { // never fold the selected tests themselves
			collect(c, 1)
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].depth != cands[j].depth {
			return cands[i].depth > cands[j].depth
		}
		return cands[i].size > cands[j].size
	})
	// fold in batches to keep this fast on big trees, then refine the last batch
	for i := 0; i < len(cands) && len(out) > maxBytes; {
		step := max(1, (len(cands)-i)/50)
		batch := cands[i:min(len(cands), i+step)]
		for _, c := range batch {
			folded[c.n] = true
		}
		next := renderCompact(nodes, folded)
		if len(next) <= maxBytes && step > 1 {
			for _, c := range batch { // overshot: fold this batch one node at a time
				delete(folded, c.n)
			}
			for _, c := range batch {
				folded[c.n] = true
				if out = renderCompact(nodes, folded); len(out) <= maxBytes {
					break
				}
			}
			break
		}
		out = next
		i += len(batch)
	}
	return out
}

func compactNode(b *strings.Builder, n *Node, depth int, folded map[*Node]bool) {
	if n.Ref != "" && len(n.Values) == 0 && len(n.Children) == 0 {
		return // a repeated helper call without own values: nothing new to show
	}
	line := compactHead(n)
	if line == "" {
		return
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString(line)
	fold := folded[n] && len(n.Children) > 0
	vals := nodeValues(n)
	if fold {
		fmt.Fprintf(b, " +%d below", countAssertions(n.Children))
		vals = subtreeValues(n, foldedValues)
	}
	if len(vals) > 0 && (n.Kind != "test" || fold) { // a folded test keeps the values checked below it
		b.WriteString(" {")
		b.WriteString(compactValues(vals))
		b.WriteString("}")
	}
	b.WriteString("\n")
	if fold {
		return
	}
	for _, c := range n.Children {
		compactNode(b, c, depth+1, folded)
	}
}

func compactHead(n *Node) string {
	pos := ""
	if n.Pos != "" {
		pos = " @" + n.Pos
	}
	label := truncate(n.Label, 100)
	switch n.Kind {
	case "test":
		return fmt.Sprintf("TEST %s%s (%d assertion(s))", n.Label, pos, n.AssertionCount)
	case "suite":
		return "SUITE " + n.Label
	case "hook":
		return "HOOK " + n.Label
	case "subtest":
		return "subtest " + label + pos
	case "helper":
		if n.Ref != "" {
			return fmt.Sprintf("> %s%s (repeat of @%s, %d assertion(s))", label, pos, n.Ref, n.Repeat)
		}
		return "> " + label + pos
	case "closure", "callback":
		return "> " + label + pos
	case "predicate":
		return "> predicate " + label + pos + ", true when:"
	case "assertion":
		return marker(n.Assertion) + " " + truncate(n.Assertion.Summary, 140) + pos
	case "eventually":
		return "~" + marker(n.Assertion) + " " + truncate(n.Assertion.Summary, 140) + pos
	case "implicit":
		return "[M] " + label + pos
	case "skip":
		return "skip: " + label + pos
	case "flaky":
		return "flaky: " + label + pos
	case "opaque":
		return "[?] " + label + pos
	default: // if, else, loop, switch, case, defer
		return truncate(n.Label, 100)
	}
}

// subtreeValues returns up to limit values of n and of the nodes below it.
func subtreeValues(n *Node, limit int) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(*Node)
	walk = func(m *Node) {
		for _, v := range nodeValues(m) {
			if !seen[v] && len(out) < limit {
				seen[v] = true
				out = append(out, v)
			}
		}
		for _, c := range m.Children {
			walk(c)
		}
	}
	walk(n)
	return out
}

func compactValues(vals []string) string {
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = fmt.Sprintf("%q", truncate(v, compactValueLen))
	}
	return strings.Join(q, ", ")
}
