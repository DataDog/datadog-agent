// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"os"
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
