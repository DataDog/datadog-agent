// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package main is the test-json-review tool which reports all failed tests from the test JSON output
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

var flakyTestFile string
var codeownersFile string
var testDirRoot string

const (
	flakyFormat = "FLAKY: %s %s"
	failFormat  = "FAIL: %s %s"
	rerunFormat = "re-ran %s %s: %s"
)

func init() {
	color.NoColor = false
	flag.StringVar(&flakyTestFile, "flakes", "", "Path to flaky test file")
	flag.StringVar(&codeownersFile, "codeowners", "", "Path to CODEOWNERS file")
	flag.StringVar(&testDirRoot, "test-root", "", "Path to test binaries")
}

func main() {
	flag.Parse()
	out, err := reviewTests("/ci-visibility/testjson/out.json", flakyTestFile, codeownersFile)
	if err != nil {
		log.Fatal(err)
	}
	if out.ReRuns != "" {
		fmt.Println(out.ReRuns)
	}
	if out.Failed == "" && out.Flaky == "" {
		fmt.Println(color.GreenString("All tests passed."))
		return
	}
	if out.Flaky != "" {
		fmt.Println(color.YellowString(out.Flaky))
	}
	if out.Failed != "" {
		fmt.Println(color.RedString(out.Failed))
		// We want to make sure the exit code is correctly set to
		// failed here, so that the CI job also fails.
		os.Exit(1)
	}
}

type testEvent struct {
	Time    time.Time // encodes as an RFC3339-format string
	Action  string
	Package string
	Test    string
	Elapsed float64 // seconds
	Output  string
}

func testKey(te testEvent) string {
	return fmt.Sprintf("%s/%s", te.Package, te.Test)
}

type reviewOutput struct {
	Failed string
	ReRuns string
	Flaky  string
}

func reviewTests(jsonFile string, flakyFile string, ownersFile string) (*reviewOutput, error) {
	jf, err := os.Open(jsonFile)
	if err != nil {
		return nil, fmt.Errorf("open %s: %s", jsonFile, err)
	}
	defer jf.Close()

	var ff io.ReadCloser
	if flakyFile != "" {
		ff, err = os.Open(flakyFile)
		if err != nil {
			return nil, fmt.Errorf("open %s: %s", flakyFile, err)
		}
		defer ff.Close()
	}

	var owners *testowners
	if ownersFile != "" {
		owners, err = newTestowners(ownersFile, testDirRoot)
		if err != nil {
			return nil, fmt.Errorf("parse codeowners: %s", err)
		}
	}

	return reviewTestsReaders(jf, ff, owners)
}

func reviewTestsReaders(jf io.Reader, ff io.Reader, owners *testowners) (*reviewOutput, error) {
	var failedTestsOut, flakyTestsOut, rerunTestsOut strings.Builder
	var kf *flake.KnownFlakyTests
	var err error
	if ff != nil {
		kf, err = flake.Parse(ff)
		if err != nil {
			return nil, fmt.Errorf("parse flakes.yaml: %s", err)
		}
	} else {
		kf = &flake.KnownFlakyTests{}
	}

	scanner := bufio.NewScanner(jf)
	testResults := make(map[string]testEvent)
	for scanner.Scan() {
		var ev testEvent
		data := scanner.Bytes()
		if err := json.Unmarshal(data, &ev); err != nil {
			return nil, fmt.Errorf("json unmarshal `%s`: %s", string(data), err)
		}
		if ev.Test == "" {
			continue
		}
		if ev.Action == "output" && flake.HasFlakyTestMarker(ev.Output) {
			kf.Add(ev.Package, ev.Test)
		}
		if ev.Action != "pass" && ev.Action != "fail" {
			continue
		}
		if res, ok := testResults[testKey(ev)]; ok {
			// If the test is already recorded as passed, it means the test
			// eventually succeeded.
			if res.Action == "pass" {
				continue
			}
			if res.Action == "fail" {
				var owner string
				if owners != nil {
					owner = owners.matchTest(ev)
				}

				rerunTestsOut.WriteString(addOwnerInformation(fmt.Sprintf(rerunFormat, ev.Package, ev.Test, ev.Action), owner))
			}
			// overwrite previously failed result
			if res.Action == "fail" && ev.Action == "pass" {
				testResults[testKey(ev)] = ev
			}
		} else {
			testResults[testKey(ev)] = ev
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("json line scan: %s", err)
	}

	failedTests := make(map[string][]string)
	for _, ev := range testResults {
		if ev.Action == "fail" {
			failedTests[ev.Package] = append(failedTests[ev.Package], ev.Test)
		}
	}

	sortedFailedPkgs := maps.Keys(failedTests)
	sort.Strings(sortedFailedPkgs)

	for _, pkg := range sortedFailedPkgs {
		tests := failedTests[pkg]
		sort.Strings(tests)

		for _, test := range tests {
			var owner string
			if owners != nil {
				owner = owners.matchTest(testEvent{Package: pkg, Test: test})
			}

			if kf.IsFlaky(pkg, test) {
				flakyTestsOut.WriteString(addOwnerInformation(fmt.Sprintf(flakyFormat, pkg, test), owner))
				continue
			}

			// check if a subtest also failed and is marked as flaky
			for _, failedTest := range tests {
				if kf.IsFlaky(pkg, failedTest) && strings.HasPrefix(failedTest, test+"/") {
					flakyTestsOut.WriteString(addOwnerInformation(fmt.Sprintf(flakyFormat, pkg, test), owner))
					continue
				}
			}

			failedTestsOut.WriteString(addOwnerInformation(fmt.Sprintf(failFormat, pkg, test), owner))
		}
	}

	return &reviewOutput{
		Failed: failedTestsOut.String(),
		ReRuns: rerunTestsOut.String(),
		Flaky:  flakyTestsOut.String(),
	}, nil
}

func addOwnerInformation(result string, owner string) string {
	if owner != "" {
		return fmt.Sprintf("%-90s [owner: %s]\n", result, owner)
	}
	return result + "\n"
}
