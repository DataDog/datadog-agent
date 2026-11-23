// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

// Test is a test of the check: the setup that it runs in, and its steps.
type Test struct {
	Name string
	// Setup is how the check runs. The zero value is the Cluster Agent leader with every collector.
	Setup Setup
	Steps []Step
}

// Setup is how the check is deployed and configured. Manifest collection, the manifest buffer and terminated
// resources are always enabled, as in their defaults.
type Setup struct {
	Mode DeploymentMode
	// DefaultCollectors leaves the collector selection to the check: discovery selects the stable collectors. By default,
	// the check configuration selects every built-in collector, so that each resource is collected.
	DefaultCollectors bool
	// OlderVersions serves the last version of each resource that has several, instead of the first one.
	OlderVersions bool
	// Instance replaces the collector selection of the check. Object.Collected ignores it, so the test selects the
	// objects of its expected payloads.
	Instance string
	// Undiscovered are the full names of the objects that API discovery does not report, until the test serves them.
	Undiscovered []string
	// BeforeConfigure changes the environment before the check is configured.
	BeforeConfigure func(t *testing.T, env *Env)
}

// DeploymentMode is where the check runs.
type DeploymentMode int

const (
	ClusterAgentLeader DeploymentMode = iota
	ClusterAgentFollower
	ClusterCheckRunner
)

// Step is a change to the environment, and everything that the check must send after it. The runner adds the cluster
// when the check leads and the nodes changed, so a step only names the objects it expects.
type Step struct {
	Name string
	// Change changes the cluster or the check before the run. Nil only runs the check.
	Change func(t *testing.T, env *Env)
	// Expect returns the payloads of the objects that the check must send after the change.
	Expect func(t *testing.T, env *Env) Payloads
	// Run is how the step runs the check. The zero value runs it until it sent the expected payloads.
	Run RunKind
	// Ignore excludes the payloads that the step cannot predict. The step gives the reason.
	Ignore func(s Sent) bool
}

// RunKind is how a step runs the check.
type RunKind int

const (
	// RunCheck runs the check until it sent the expected payloads.
	RunCheck RunKind = iota
	// RunFails runs the check, which must return an error.
	RunFails
)

// RunTests runs each test as a subtest.
func (s *Suite) RunTests(t *testing.T, tests ...Test) {
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) { s.runTest(t, test) })
	}
}

// runTest creates the environment of the test, and runs its steps in order.
func (s *Suite) runTest(t *testing.T, test Test) {
	env := NewEnv(t, s.SenderManager, test.Setup)
	for _, step := range test.Steps {
		env.runStep(t, step)
	}
}

// runStep changes the environment, runs the check until it sent every expected payload, and compares everything sent
// with the expected payloads.
func (env *Env) runStep(t *testing.T, step Step) {
	if step.Change != nil {
		step.Change(t, env)
	}
	want := step.Expect(t, env)
	// An instance configuration replaces the collector selection, which includes the cluster.
	if env.leader && env.nodesChanged && step.Run == RunCheck && !env.discovery.failing() && env.Setup.Instance == "" {
		want = want.With(env.clusterPayloads())
		env.nodesChanged = false
	}
	run := env.run
	if step.Run == RunFails {
		run = func() []Sent {
			assert.Error(t, runWithTimeout(t, env.Check.Run))
			return env.sink.take()
		}
	}
	sent := eventually(t, run, func(sent []Sent) bool {
		counts := map[payloadKey]int{}
		for _, s := range sent {
			counts[payloadKey{s.UID, s.Metadata, s.Terminated}]++
		}
		for key, payloads := range want {
			if counts[key] < len(payloads) {
				return false
			}
		}
		return true
	})
	if diff := cmp.Diff(want, receivedPayloads(t, sent, step.Ignore)); diff != "" {
		t.Errorf("step %q: payloads (-want +got):\n%s", step.Name, diff)
	}
	env.assertNoSensitiveData(t, sent)
}

// eventually runs the check until the condition holds on everything sent, as informers report changes
// asynchronously, and returns everything sent.
func eventually(t *testing.T, run func() []Sent, condition func([]Sent) bool) []Sent {
	t.Helper()
	var sent []Sent
	deadline := time.Now().Add(5 * time.Second)
	for {
		sent = append(sent, run()...)
		if condition(sent) || time.Now().After(deadline) {
			// Let late events arrive, so that duplicates show up.
			time.Sleep(30 * time.Millisecond)
			return append(sent, run()...)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// runWithTimeout returns the error of the function, and fails the test when the function blocks, as a blocked check
// run would otherwise hang the suite. The function runs in another goroutine, so it must not fail the test itself.
func runWithTimeout(t *testing.T, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the check is blocked")
		return nil
	}
}
