// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

// Package suite tests the orchestrator check as a whole: the real check runs against a fake API server that serves an
// object of every resource, and each test compares what the check sends with the payloads it expects. A new collector
// is in every test once Resources lists it and it has a fixture, and every test fails until then.
//
// The suite has four concepts:
//
//	Test = Setup + Steps    Setup ──creates──▶ Env ◀── each Step: change the Env, run the check, compare what it sent
//
// A Test is a Setup and a list of Steps. A Setup is how the check is deployed and configured. An Env is the check,
// the fake cluster and the other mocks of the Setup, with the Objects of the cluster. A Step changes the Env, runs the
// check until it sent the Payloads that the Step expects, and compares everything sent with them.
//
// The cases package registers the tests, one group for each area, and the checktest package runs them.
package suite

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// Suite holds what the tests of the check share, as creating it takes longer than an environment.
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
	s := &Suite{SenderManager: mocksender.CreateDefaultDemultiplexer(t)}
	sorted := slices.SortedFunc(slices.Values(groups), func(a, b group) int { return strings.Compare(a.title, b.title) })
	for _, g := range sorted {
		t.Run(g.title, func(t *testing.T) { g.fn(t, s) })
	}
}
