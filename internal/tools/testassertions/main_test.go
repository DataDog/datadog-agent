// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"path/filepath"
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

func TestUnknownTest(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "sample"))
	require.NoError(t, err)
	l := newLoader(dir)
	_, err = selectTests(l, l.load(dir, true), []string{"TestDoesNotExist"}, "")
	assert.Error(t, err)
}
