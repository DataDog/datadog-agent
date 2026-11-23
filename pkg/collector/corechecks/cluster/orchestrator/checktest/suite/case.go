// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func newCheckSuite(t *testing.T) *Suite {
	return &Suite{SenderManager: mocksender.CreateDefaultDemultiplexer(t)}
}

// A case is a sequence of steps that runs in each of its variants. Each step changes the cluster or the check, runs
// the check until it sent what the step expects, and compares everything sent with the expected payloads once.

// Case is a test of the check.
type Case struct {
	Name string
	// Variants are where the case runs. Nil runs it on the Cluster Agent leader with each manifest pipeline.
	Variants []Variant
	// Configure changes the configuration of each variant: the check instance, discovery or dependencies.
	Configure func(cfg *ScenarioConfig)
	Steps     []Step
}

// Variant is a deployment mode and configuration of the check, and where its payloads are read.
type Variant struct {
	Mode DeploymentMode
	Cfg  ScenarioConfig
}

func (v Variant) String() string {
	return string(v.Mode) + "/" + v.Cfg.String()
}

// PipelineVariants returns the variants of a deployment mode with each manifest pipeline.
func PipelineVariants(mode DeploymentMode) []Variant {
	return []Variant{{Mode: mode}, {Mode: mode, Cfg: ScenarioConfig{DedicatedPipeline: true}}}
}

// Step is a change to the cluster or the check, and everything that the check must send after it.
type Step struct {
	Name string
	// Act changes the cluster or the check before the run. Nil only runs the check.
	Act func(t *testing.T, sc *Scenario)
	// Want returns every payload that the step must send.
	Want func(t *testing.T, sc *Scenario) ExpectedPayloads
	Run  RunKind
	// Ignore excludes the payloads that the step cannot predict. The step gives the reason.
	Ignore func(s SentResource) bool
}

// RunKind is how a step runs the check.
type RunKind int

const (
	// RunCheck runs the check until it sent the expected payloads.
	RunCheck RunKind = iota
	// CancelCheck cancels the check, which sends what it queued.
	CancelCheck
	// RunFails runs the check, which must return an error.
	RunFails
)

// FirstRun runs the check on the fixtures: every collected object is sent once.
var FirstRun = Step{Name: "first run", Want: func(t *testing.T, sc *Scenario) ExpectedPayloads { return sc.LiveAll(t) }}

func Nothing(*testing.T, *Scenario) ExpectedPayloads { return ExpectedPayloads{} }

// RunCases runs each case as a subtest.
func (cs *Suite) RunCases(t *testing.T, cases ...Case) {
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) { cs.RunCase(t, c) })
	}
}

// RunCase runs the case in each variant, and returns what each step sent, by variant.
func (cs *Suite) RunCase(t *testing.T, c Case) map[string][][]SentResource {
	variants := c.Variants
	if variants == nil {
		variants = PipelineVariants(ClusterAgentLeader)
	}
	sent := map[string][][]SentResource{}
	for _, v := range variants {
		t.Run(v.String(), func(t *testing.T) { sent[v.String()] = cs.runVariant(t, c, v) })
	}
	return sent
}

func (cs *Suite) runVariant(t *testing.T, c Case, v Variant) [][]SentResource {
	cfg := v.Cfg
	if c.Configure != nil {
		c.Configure(&cfg)
	}
	sc := NewScenario(t, cs.SenderManager, v.Mode, cfg)

	var sent [][]SentResource
	for _, s := range c.Steps {
		if s.Act != nil {
			s.Act(t, sc)
		}
		want := s.Want(t, sc)
		got := sc.runStep(t, s, want)
		if diff := cmp.Diff(want, receivedPayloads(t, got, s.Ignore)); diff != "" {
			t.Errorf("step %q: payloads (-want +got):\n%s", s.Name, diff)
		}
		assertNoSensitiveData(t, sc.Expected, got)
		sent = append(sent, got)
	}
	return sent
}

// runStep runs the check until it sent every expected payload, and returns everything sent.
func (sc *Scenario) runStep(t *testing.T, s Step, want ExpectedPayloads) []SentResource {
	var run func() []SentResource
	switch s.Run {
	case RunCheck:
		run = sc.Run
	case CancelCheck:
		run = sc.Cancel
	case RunFails:
		run = func() []SentResource {
			runWithTimeout(t, func() { assert.Error(t, sc.Chk.Run()) })
			return sc.Mocks.Sink.take(t)
		}
	}
	return eventually(t, run, func(sent []SentResource) bool {
		counts := map[string]int{}
		for _, r := range sent {
			counts[payloadKey(r.UID, r.Metadata, r.Terminated)]++
		}
		for key, payloads := range want {
			if counts[key] < len(payloads) {
				return false
			}
		}
		return true
	})
}
