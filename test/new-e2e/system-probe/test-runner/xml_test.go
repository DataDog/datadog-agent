// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const xmldoc = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="2" failures="0" errors="0" time="17.516333">
	<testsuite tests="2" failures="0" time="0.021000" name="pkg/collector/corechecks/ebpf/probe/ebpfcheck" timestamp="2023-11-13T19:15:59Z">
		<properties>
			<property name="go.version" value="unknown"></property>
		</properties>
		<testcase classname="pkg/collector/corechecks/ebpf/probe/ebpfcheck" name="TestEBPFPerfBufferLength" time="0.000000">
			<skipped message="=== RUN   TestEBPFPerfBufferLength&#xA;    version_linux.go:17: skipping test; it requires kernel version 5.5.0 or later, running on: 4.14.255&#xA;--- SKIP: TestEBPFPerfBufferLength (0.00s)&#xA;"></skipped>
		</testcase>
		<testcase classname="pkg/collector/corechecks/ebpf/probe/ebpfcheck" name="TestMinMapSize" time="0.000000">
			<skipped message="=== RUN   TestMinMapSize&#xA;    version_linux.go:17: skipping test; it requires kernel version 5.5.0 or later, running on: 4.14.255&#xA;--- SKIP: TestMinMapSize (0.00s)&#xA;"></skipped>
		</testcase>
	</testsuite>
</testsuites>`

const retriedXMLDoc = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="6" failures="4" errors="0" time="0.016097">
	<testsuite tests="6" failures="4" time="0.002000" name="flakydemo" timestamp="2026-09-19T10:18:42Z">
		<testcase classname="flakydemo" name="TestFlaky" time="0.000000">
			<failure message="Failed" type="">first attempt failed</failure>
		</testcase>
		<testcase classname="flakydemo" name="TestFlaky" time="0.000000"></testcase>
		<testcase classname="flakydemo" name="TestAlwaysPass" time="0.000000"></testcase>
		<testcase classname="flakydemo" name="TestAlwaysFail" time="0.000000">
			<failure message="Failed" type="">attempt 1</failure>
		</testcase>
		<testcase classname="flakydemo" name="TestAlwaysFail" time="0.000000">
			<failure message="Failed" type="">re-run 1</failure>
		</testcase>
		<testcase classname="flakydemo" name="TestAlwaysFail" time="0.000000">
			<failure message="Failed" type="">re-run 2</failure>
		</testcase>
	</testsuite>
</testsuites>`

func TestXMLDecode(t *testing.T) {
	buf := bytes.NewBufferString(xmldoc)
	var suites JUnitTestSuites
	err := decode(buf, &suites)
	if err != nil {
		t.Fatal(err)
	}
	if len(suites.Suites) != 1 {
		t.Fatalf("expected 1 testsuite, got %d", len(suites.Suites))
	}
	suite := suites.Suites[0]
	if len(suite.TestCases) != 2 {
		t.Fatalf("expected 2 testcases, got %d", len(suite.TestCases))
	}
}

func TestAddProperties(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.xml")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	_, err = f.WriteString(xmldoc)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	err = addProperties(path, map[string]string{
		"dd_tags[os.platform]":     "linux",
		"dd_tags[os.name]":         "ubuntu-22.04",
		"dd_tags[os.architecture]": "arm64",
		"dd_tags[os.version]":      "5.15.0-73-generic",
	})
	if err != nil {
		t.Fatal(err)
	}

	var suites JUnitTestSuites
	err = openAndDecode(path, &suites)
	if err != nil {
		t.Fatal(err)
	}
	if len(suites.Suites) != 1 {
		t.Fatalf("expected 1 testsuite, got %d", len(suites.Suites))
	}
	for _, s := range suites.Suites {
		if len(s.Properties) < 4 {
			t.Fatalf("expected at least 4 properties, got %d", len(s.Properties))
		}
	}
}

func TestMarkRetriedTestCases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.xml")
	if err := os.WriteFile(path, []byte(retriedXMLDoc), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := markRetriedTestCases(path); err != nil {
		t.Fatal(err)
	}

	var suites JUnitTestSuites
	if err := openAndDecode(path, &suites); err != nil {
		t.Fatal(err)
	}

	// Expected agent_was_retried values, in the order each named test's attempts appear above:
	// every attempt is retried except the deciding, last one.
	want := map[string][]string{
		"TestFlaky":      {"true", "false"},
		"TestAlwaysPass": {"false"},
		"TestAlwaysFail": {"true", "true", "false"},
	}
	seen := map[string]int{}
	for _, tc := range suites.Suites[0].TestCases {
		i := seen[tc.Name]
		seen[tc.Name] = i + 1
		if got, want := tc.WasRetried, want[tc.Name][i]; got != want {
			t.Errorf("testcase %s attempt %d: got was_retried=%q, want %q", tc.Name, i, got, want)
		}
	}
}
