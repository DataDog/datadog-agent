// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build stresstests

package tests

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var (
	coreID  int
	nbRuns  int
	nbSkips int
	host    string
)

//go:embed latency/bin
var benchLatencyhFS embed.FS

// modified version of testModule.CreateWithOption, to be able to call it without testing module
func CreateWithOptions(tb testing.TB, filename string, user, group, mode int) (string, unsafe.Pointer, error) {
	var macros []*rules.MacroDefinition
	var rules []*rules.RuleDefinition

	if err := initLogger(); err != nil {
		return "", nil, err
	}

	st, err := newSimpleTest(tb, macros, rules, "")
	if err != nil {
		return "", nil, err
	}

	testFile, testFilePtr, err := st.Path(filename)
	if err != nil {
		return testFile, testFilePtr, err
	}

	// Create file
	f, err := os.OpenFile(testFile, os.O_CREATE, os.FileMode(mode))
	if err != nil {
		return "", nil, err
	}
	f.Close()

	// Chown the file
	err = os.Chown(testFile, user, group)
	return testFile, testFilePtr, err
}

// load embedded binary
func loadBenchLatencyBin(tb testing.TB, binary string) (string, error) {
	testerBin, err := benchLatencyhFS.ReadFile(fmt.Sprintf("latency/bin/%s", binary))
	if err != nil {
		return "", err
	}

	perm := 0o700
	binPath, _, _ := CreateWithOptions(tb, binary, -1, -1, perm)

	f, err := os.OpenFile(binPath, os.O_WRONLY|os.O_CREATE, os.FileMode(perm))
	if err != nil {
		return "", err
	}

	if _, err = f.Write(testerBin); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	return binPath, nil
}

// bench induced latency for DNS req
func benchLatencyDNS(t *testing.T, rule *rules.RuleDefinition, executable string) {
	// do not load module if no rule is provided
	if rule != nil {
		var ruleDefs []*rules.RuleDefinition
		ruleDefs = append(ruleDefs, rule)
		test, err := newTestModule(t, nil, ruleDefs,
			testOpts{eventsCountThreshold: 1000000})
		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()
	}

	// load bench binary
	executable, err := loadBenchLatencyBin(t, executable)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(executable)

	// exec the bench tool
	cmd := exec.Command("taskset", "-c", fmt.Sprint(coreID),
		executable, host, fmt.Sprint(nbRuns), fmt.Sprint(nbSkips))
	output, err := cmd.CombinedOutput()
	t.Logf("Output:\n%s", output)
	if err != nil {
		t.Fatal(err)
	}
}

// goal: measure the induced latency when no kprobes/tc are loaded
func TestLatency_DNSNoKprobe(t *testing.T) {
	benchLatencyDNS(t, nil, "bench_net_DNS")
}

// goal: measure the induced latency when kprobes are loaded, but without a matching rule
func TestLatency_DNSNoRule(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`dns.question.name == "%s.nope"`, host),
	}
	benchLatencyDNS(t, rule, "bench_net_DNS")
}

// goal: measure the induced latency when kprobes are loaded, with a matching rule
func TestLatency_DNS(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`dns.question.name == "%s"`, host),
	}
	benchLatencyDNS(t, rule, "bench_net_DNS")
}

func init() {
	flag.IntVar(&nbRuns, "nbruns", 100100, "number of runs to perform")
	flag.IntVar(&nbSkips, "nbskips", 100, "number of first runs to skip from measurement")
	flag.IntVar(&coreID, "coreid", 0, "CPU core ID to pin the bench program")
	flag.StringVar(&host, "host", "google.com", "Host to query")
}
