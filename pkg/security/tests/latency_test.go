// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build stresstests
// +build stresstests

package tests

import (
	"flag"
	"fmt"
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var (
	coreID  int
	nbRuns  int
	nbSkips int
	host    string
)

// bench induced latency for DNS req
func benchLatencyDNS(t *testing.T, rule *rules.RuleDefinition, executable string) {
	// do not load module if no rule is provided
	if rule != nil {
		var ruleDefs []*rules.RuleDefinition
		ruleDefs = append(ruleDefs, rule)
		test, err := newTestModule(t, nil, ruleDefs,
			testOpts{enableNetwork: true, eventsCountThreshold: 1000000})
		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()
	}

	// exec the bench tool
	cmd := exec.Command("taskset", "-c", fmt.Sprint(coreID),
		executable, host, fmt.Sprint(nbRuns), fmt.Sprint(nbSkips))
	output, err := cmd.CombinedOutput()
	t.Log("Output:\n%s", output)
	if err != nil {
		t.Fatal(err)
	}
}

// goal: measure the induced latency when no kprobes/tc are loaded
func TestLatency_DNSNoKprobe(t *testing.T) {
	executable := "pkg/security/tests/latency/bench_net_DNS"
	benchLatencyDNS(t, nil, executable)
}

// goal: measure the induced latency when kprobes are loaded, but without a matching rule
func TestLatency_DNSNoRule(t *testing.T) {
	executable := "pkg/security/tests/latency/bench_net_DNS"
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`dns.question.name == "%s.nope"`, host),
	}
	benchLatencyDNS(t, rule, executable)
}

// goal: measure the induced latency when kprobes are loaded, with a matching rule
func TestLatency_DNS(t *testing.T) {
	executable := "pkg/security/tests/latency/bench_net_DNS"
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`dns.question.name == "%s"`, host),
	}
	benchLatencyDNS(t, rule, executable)
}

func init() {
	flag.IntVar(&nbRuns, "nbruns", 100100, "number of runs to perform")
	flag.IntVar(&nbSkips, "nbskips", 100, "number of first runs to skip from measurement")
	flag.IntVar(&coreID, "coreid", 0, "CPU core ID to pin the bench program")
	flag.StringVar(&host, "host", "google.com", "Host to query")
}
