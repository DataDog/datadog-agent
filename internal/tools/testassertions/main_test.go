// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extractSample(t *testing.T, names ...string) []*Node {
	dir, err := filepath.Abs(filepath.Join("testdata", "sample"))
	require.NoError(t, err)
	l := newLoader(dir)
	pkg := l.load(dir, true)
	ex := newExtractor(l, pkg, followModule, 8)
	tests, err := selectTests(l, pkg, names, "")
	require.NoError(t, err)
	nodes := make([]*Node, 0, len(tests))
	for _, fi := range tests {
		nodes = append(nodes, ex.testNode(fi))
	}
	return nodes
}

// flatten returns "kind:summary-or-label" for every node, depth first.
func flatten(nodes []*Node) []string {
	var out []string
	var walk func(n *Node)
	walk = func(n *Node) {
		s := n.Kind + ":" + n.Label
		if n.Assertion != nil {
			s = n.Kind + ":" + n.Assertion.Summary
		}
		out = append(out, s)
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return out
}

func TestSuiteExpansion(t *testing.T) {
	got := flatten(extractSample(t, "TestLinuxSuite"))
	assert.Equal(t, []string{
		"test:TestLinuxSuite",
		"suite:linuxSuite",
		"test:(linuxSuite).TestCondition",
		"eventually:eventually, the condition below returns true (timeout time.Minute, every time.Second)",
		"assertion:s.Env().Agent.Client.IsReady()",
		"implicit:s.Env().RemoteHost.MustExecute(\"ls /etc/datadog-agent\")",
		"helper:withRetry(s.T(), func(t *testing.T) { … })",
		"closure:fn(t)",
		"if:if t == nil",
		"assertion:test fails: no t",
		"hook:(baseSuite).SetupSuite",
		"assertion:setup() is nil (no error)",
		"test:(baseSuite).TestHostname",
		"assertion:err is nil (no error)",
		"assertion:out == expectedHost",
		"eventually:eventually, all assertions below pass in a single attempt (timeout 2 * time.Minute, every 10 * time.Second)",
		"assertion:err is nil (no error)",
		"if:if the assertion above passed",
		"loop:for each m in metrics",
		"helper:checkTags(c, m.Tags)",
		"assertion:tags contains \"host:\" + expectedHost",
	}, got)
}

func TestAssertionDetails(t *testing.T) {
	nodes := extractSample(t, "TestHostname")
	require.Len(t, nodes, 1)
	n := nodes[0].Children[1]
	require.NotNil(t, n.Assertion)
	assert.Equal(t, "s", n.Assertion.Library)
	assert.Equal(t, "hostname mismatch", n.Assertion.Message)
	assert.False(t, n.Assertion.Fatal)
	assert.Contains(t, n.Assertion.Origins, Origin{Name: "out", From: "← s.Env().Agent.Client.Hostname()"})
	assert.Contains(t, n.Assertion.Origins, Origin{Name: "expectedHost", From: `(package-level) = "my-host"`})
	assert.True(t, nodes[0].Children[0].Assertion.Fatal)
}

func TestTreeOutput(t *testing.T) {
	var buf bytes.Buffer
	(&treePrinter{w: &buf, brief: true}).print(extractSample(t, "TestHostname")[0])
	assert.Contains(t, buf.String(), "5 assertion(s): 1 fatal (require/Fatal), 1 polling block(s), 1 under a condition/loop; 1 helper(s) followed")
}

func TestConstructorReturningInterface(t *testing.T) {
	got := flatten(extractSample(t, "TestFleetStyle"))
	require.GreaterOrEqual(t, len(got), 4)
	assert.Equal(t, []string{
		"test:TestFleetStyle",
		"helper:runOnPlatforms(t, newConfigSuite, []string{\"ubuntu\", \"debian\"})",
		"loop:for each platform in platforms",
		"subtest:platform",
		"suite:configSuite",
	}, got[:5])
}

func TestRepeatedHelpersAndValues(t *testing.T) {
	nodes := extractSample(t, "configSuite.TestSections")
	require.Len(t, nodes, 1)
	test := nodes[0]
	require.Len(t, test.Children, 2)

	// first call: expanded, with values traced through the range loop and the struct field
	first := test.Children[0].Children[0]
	require.Equal(t, "helper", first.Kind)
	require.Len(t, first.Children, 2)
	assert.Equal(t, []string{"Collector"}, first.Children[0].Assertion.Values)
	contains := first.Children[1].Children[0].Assertion
	assert.Equal(t, []string{"status output", "Running Checks", "logs_enabled"}, contains.Values,
		"only sec.shouldContain values, constants resolved, not sec.name")

	// second call: collapsed into a reference, but still carrying its own values
	second := test.Children[1]
	assert.Equal(t, "helper", second.Kind)
	assert.NotEmpty(t, second.Ref)
	assert.Equal(t, 2, second.Repeat)
	assert.Empty(t, second.Children)
	assert.Equal(t, []string{"Forwarder", "Transactions"}, second.Values)

	assert.ElementsMatch(t, []string{"Collector", "status output", "Running Checks", "logs_enabled", "Forwarder", "Transactions"}, test.Values)
}

func TestPredicateInStructLiteral(t *testing.T) {
	got := flatten(extractSample(t, "TestGenericRun"))
	assert.Equal(t, []string{
		"test:TestGenericRun",
		"suite:orchSuite",
		"test:(orchSuite).TestPod",
		"helper:expectResource{test: func(kind string) bool { … }, message: \"find a pod\"}.Assert(s.T(), nil)",
		"loop:for each k in kinds",
		"predicate:e.test(k)",
		"assertion:kind == \"Pod\"",
		"assertion:test fails: \"failed to \" + e.message",
	}, got)
	failure := extractSample(t, "orchSuite.TestPod")[0].Children[0].Children[1].Assertion
	assert.Equal(t, []string{"find a pod"}, failure.Values)
}

func TestEntryPointAggregatesItsSuites(t *testing.T) {
	nodes := extractSample(t, "TestFleetStyle")
	require.Len(t, nodes, 1)
	root := nodes[0]
	var methods []*Node
	var walk func(*Node)
	walk = func(n *Node) {
		for _, c := range n.Children {
			if c.Kind == "test" {
				methods = append(methods, c)
			} else {
				walk(c)
			}
		}
	}
	walk(root)
	require.NotEmpty(t, methods)
	total := 0
	for _, m := range methods {
		total += m.AssertionCount
		for _, v := range m.Values {
			assert.Contains(t, root.Values, v, "values of %s are part of the entry point's", m.Label)
		}
	}
	assert.Equal(t, total, root.AssertionCount)
	assert.Contains(t, root.Values, "logs_enabled")
}

func TestSmallRepeatedHelperIsCollapsed(t *testing.T) {
	test := extractSample(t, "configSuite.TestSmallHelperTwice")[0]
	require.Len(t, test.Children, 2)
	first, second := test.Children[0], test.Children[1]
	assert.Empty(t, first.Ref)
	assert.Len(t, first.Children, 1)
	assert.Equal(t, first.Pos, second.Ref)
	assert.Equal(t, 1, second.Repeat)
	assert.Equal(t, []string{"datadog-agent-sysprobe"}, second.Values, "the collapsed call keeps its own values")
	assert.Equal(t, 2, test.AssertionCount)
}

func TestCompactBudget(t *testing.T) {
	nodes := extractSample(t, "TestLinuxSuite", "TestFleetStyle")
	full := renderCompact(nodes, nil)
	assert.Equal(t, full, renderCompactBudget(nodes, len(full)), "nothing folded when it fits")
	assert.NotContains(t, full, "│", "ASCII only")
	assert.Contains(t, full, `{"Collector"}`, "values inline")

	budget := len(full) * 2 / 3
	folded := renderCompactBudget(nodes, budget)
	assert.LessOrEqual(t, len(folded), budget)
	assert.Contains(t, folded, " below")
	assert.Contains(t, folded, "logs_enabled", "folded lines keep the values found below them")
	assert.Contains(t, folded, "= 9 assertion(s)", "totals are unchanged")
	assert.Contains(t, folded, "= 6 assertion(s)")
}

func TestSuitesPickedFromALoop(t *testing.T) {
	for _, name := range []string{"TestSuitesFromTable", "TestSuitesFromSlice"} {
		t.Run(name, func(t *testing.T) {
			nodes := extractSample(t, name)
			require.Len(t, nodes, 1)
			var suites []string
			for _, line := range flatten(nodes) {
				if strings.HasPrefix(line, "suite:") {
					suites = append(suites, strings.TrimPrefix(line, "suite:"))
				}
			}
			assert.Equal(t, []string{"amiSuite", "archSuite"}, suites)
			assert.Equal(t, 2, nodes[0].AssertionCount)
			assert.Subset(t, nodes[0].Values, []string{"ami-123", "arm64"})
		})
	}
}

func TestUnknownTest(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "sample"))
	require.NoError(t, err)
	l := newLoader(dir)
	_, err = selectTests(l, l.load(dir, true), []string{"TestDoesNotExist"}, "")
	assert.Error(t, err)
}
