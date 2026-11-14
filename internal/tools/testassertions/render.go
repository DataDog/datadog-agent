// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type treePrinter struct {
	w     io.Writer
	brief bool
}

type stats struct {
	assertions, fatal, polling, conditional, opaque, helpers, implicit, skips, flaky int
	repeated, repeatedCalls                                                          int
}

func (p *treePrinter) print(n *Node) {
	p.node(n, "", true, true, nil)
	var s stats
	countStats(n, &s, false)
	// the total counts collapsed repeats, like -list and assertionCount in -json
	fmt.Fprintf(p.w, "\n  %d assertion(s)", s.assertions+s.repeated)
	if s.repeatedCalls > 0 {
		fmt.Fprintf(p.w, " (%d shown, %d more in %d repeated helper call(s) collapsed ↺)", s.assertions, s.repeated, s.repeatedCalls)
	}
	fmt.Fprintf(p.w, ": %d fatal (require/Fatal), %d polling block(s), %d under a condition/loop; %d helper(s) followed",
		s.fatal, s.polling, s.conditional, s.helpers)
	if s.skips > 0 {
		fmt.Fprintf(p.w, "; %d skip condition(s) ⤼", s.skips)
	}
	if s.flaky > 0 {
		fmt.Fprintf(p.w, "; %d flake marker(s) ~", s.flaky)
	}
	if s.implicit > 0 {
		fmt.Fprintf(p.w, "; %d implicit Must* check(s) [M]", s.implicit)
	}
	if s.opaque > 0 {
		fmt.Fprintf(p.w, "; %d call(s) not followed that may assert [?]", s.opaque)
	}
	fmt.Fprintln(p.w)
}

func countStats(n *Node, s *stats, conditional bool) {
	switch n.Kind {
	case "assertion":
		s.assertions++
		if n.Assertion.Fatal {
			s.fatal++
		}
		if conditional {
			s.conditional++
		}
	case "eventually":
		s.assertions++
		s.polling++
		if n.Assertion.Fatal {
			s.fatal++
		}
		if conditional {
			s.conditional++
		}
	case "opaque":
		s.opaque++
	case "implicit":
		s.implicit++
	case "skip":
		s.skips++
	case "flaky":
		s.flaky++
	case "helper", "closure", "predicate":
		s.helpers++
		if n.Ref != "" {
			s.repeated += n.Repeat
			s.repeatedCalls++
		}
	case "if", "else", "loop", "case":
		conditional = true
	case "suite":
		// each test of the suite is reported on its own line in the tree
	}
	for _, c := range n.Children {
		countStats(c, s, conditional)
	}
}

func (p *treePrinter) node(n *Node, prefix string, last, top bool, parentVals []string) {
	connector, childPrefix := "├── ", prefix+"│   "
	if last {
		connector, childPrefix = "└── ", prefix+"    "
	}
	if top {
		connector, childPrefix = "", prefix
	}
	fmt.Fprintln(p.w, prefix+connector+head(n, p.brief))

	detailPrefix := childPrefix + "    "
	if len(n.Children) > 0 {
		detailPrefix = childPrefix + "│   "
	}
	if !p.brief {
		for _, d := range details(n) {
			fmt.Fprintln(p.w, detailPrefix+d)
		}
	}
	// the concrete values are what a change is usually about: show them in brief mode too
	vals := nodeValues(n)
	shown := parentVals // the closest values printed above this node
	// assertions always show their values; intermediate nodes only when they differ from above
	if len(vals) > 0 && (n.Kind != "test" || !p.brief) && (n.Assertion != nil || !slices.Equal(vals, parentVals)) {
		shown = vals
		label := "values"
		if n.Kind == "test" {
			label = "all values checked"
		}
		fmt.Fprintln(p.w, detailPrefix+"· "+label+": "+quoteAll(vals))
	}
	for i, c := range n.Children {
		p.node(c, childPrefix, i == len(n.Children)-1, false, shown)
	}
}

func marker(a *Assertion) string {
	if a.Fatal {
		return "[R]"
	}
	return "[A]"
}

func head(n *Node, brief bool) string {
	def := n.Def
	if brief {
		def = filepath.Base(def) // the full path is in the default and -json outputs
	}
	switch n.Kind {
	case "test":
		if len(n.Children) == 0 {
			return fmt.Sprintf("TEST %s  (%s)  — no assertion found", n.Label, n.Pos)
		}
		return fmt.Sprintf("TEST %s  (%s)", n.Label, n.Pos)
	case "suite":
		return fmt.Sprintf("SUITE %s  (%s, run @%s)", n.Label, n.Def, n.Pos)
	case "hook":
		return fmt.Sprintf("HOOK %s  (%s)", n.Label, n.Pos)
	case "subtest":
		return fmt.Sprintf("▸ subtest %s  @%s", n.Label, n.Pos)
	case "helper":
		if n.Ref != "" {
			if brief {
				return fmt.Sprintf("↺ %s  @%s  — as expanded @%s (%d assertion(s))", n.Label, n.Pos, n.Ref, n.Repeat)
			}
			return fmt.Sprintf("↺ %s  @%s  — same checks as the call expanded @%s (%d assertion(s))", n.Label, n.Pos, n.Ref, n.Repeat)
		}
		return fmt.Sprintf("↳ %s  @%s  (def %s)", n.Label, n.Pos, def)
	case "closure":
		return fmt.Sprintf("↳ closure %s  @%s", n.Label, n.Pos)
	case "predicate":
		return fmt.Sprintf("↳ predicate %s  @%s  — decides the enclosing if; true when it returns:", n.Label, n.Pos)
	case "callback":
		return fmt.Sprintf("↳ %s  @%s", n.Label, n.Pos)
	case "eventually":
		return fmt.Sprintf("⟳ %s %s  @%s", marker(n.Assertion), n.Assertion.Summary, n.Pos)
	case "assertion":
		return fmt.Sprintf("%s %s  @%s", marker(n.Assertion), n.Assertion.Summary, n.Pos)
	case "skip":
		return fmt.Sprintf("⤼ %s  @%s", n.Label, n.Pos)
	case "flaky":
		return fmt.Sprintf("~ %s  @%s", n.Label, n.Pos)
	case "implicit":
		return fmt.Sprintf("[M] %s succeeds (Must* fails the test otherwise)  @%s", n.Label, n.Pos)
	case "opaque":
		s := fmt.Sprintf("[?] %s  @%s  — not followed, may assert", n.Label, n.Pos)
		if n.Def != "" {
			s += " (def " + n.Def + ", use -follow=repo)"
		}
		return s
	default:
		return fmt.Sprintf("%s  @%s", n.Label, n.Pos)
	}
}

func details(n *Node) []string {
	a := n.Assertion
	if a == nil {
		return nil
	}
	var out []string
	if n.Kind == "assertion" {
		out = append(out, "· "+a.Call)
	}
	if a.Message != "" {
		out = append(out, "· msg: "+strings.ReplaceAll(a.Message, "\n", `\n`))
	}
	for _, o := range a.Origins {
		out = append(out, fmt.Sprintf("· %s %s", o.Name, o.From))
	}
	return out
}

func nodeValues(n *Node) []string {
	if n.Assertion != nil {
		return n.Assertion.Values
	}
	return n.Values
}

func quoteAll(vals []string) string {
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = strconv.Quote(v)
	}
	return strings.Join(q, ", ")
}

func printList(w io.Writer, nodes []*Node) {
	for _, n := range nodes {
		var hasSuite func(*Node) bool
		hasSuite = func(n *Node) bool {
			for _, c := range n.Children {
				if c.Kind == "suite" || hasSuite(c) {
					return true
				}
			}
			return false
		}
		if hasSuite(n) {
			fmt.Fprintf(w, "%s  (%s)  — %d assertion(s) in total\n", n.Label, n.Pos, n.AssertionCount)
		} else {
			fmt.Fprintf(w, "%s  (%s)  — %d assertion(s)\n", n.Label, n.Pos, n.AssertionCount)
		}
		var walk func(n *Node, indent string)
		walk = func(n *Node, indent string) {
			for _, c := range n.Children {
				switch c.Kind {
				case "suite":
					fmt.Fprintf(w, "%ssuite %s\n", indent, c.Label)
					walk(c, indent+"  ")
				case "test":
					fmt.Fprintf(w, "%s%s  — %d assertion(s)\n", indent, c.Label, c.AssertionCount)
				default:
					walk(c, indent)
				}
			}
		}
		walk(n, "  ")
	}
}
