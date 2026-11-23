// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

// Package suite runs the tests of the orchestrator check as a whole: the real check runs against a fake API server
// that serves a fixture of every registry entry, and the tests check what it passes to a mock sender. A new registry
// entry is covered by every test as soon as it has a fixture, and the registry tests fail until it has one.
//
// The cases package registers the tests, one group for each area, and the checktest package runs them.
package suite

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// Suite holds what the tests of the check share, as starting it takes longer than a scenario.
type Suite struct {
	SenderManager sender.SenderManager
}

// group is a group of tests that a file of the cases package registers.
type group struct {
	title string
	fn    func(t *testing.T, s *Suite)
}

var groups []group

// Register adds a group of tests to the suite. Each file of the cases package registers the group of its area in init.
func Register(title string, fn func(t *testing.T, s *Suite)) {
	groups = append(groups, group{title: title, fn: fn})
}

// Run runs every registered group as a subtest, by title.
func Run(t *testing.T) {
	require.NotEmpty(t, groups, "no group is registered: the test must import the cases package")
	s := newCheckSuite(t)
	sorted := slices.SortedFunc(slices.Values(groups), func(a, b group) int { return strings.Compare(a.title, b.title) })
	for _, g := range sorted {
		t.Run(g.title, func(t *testing.T) { g.fn(t, s) })
	}
}
