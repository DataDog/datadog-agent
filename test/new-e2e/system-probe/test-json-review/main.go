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
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

var flakyTestFile string

const (
	flakyFormat = "FLAKY: %s %s\n"
	failFormat  = "FAIL: %s %s\n"
	rerunFormat = "re-ran %s %s: %s\n"
)

func init() {
	color.NoColor = false
	flag.StringVar(&flakyTestFile, "flakes", "", "Path to flaky test file")
}

func main() {
	flag.Parse()
	out, err := reviewTests("/ci-visibility/testjson/out.json", flakyTestFile)
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

func testKey(test, pkg string) string {
	return fmt.Sprintf("%s/%s", test, pkg)
}

type reviewOutput struct {
	Failed string
	ReRuns string
	Flaky  string
}

func reviewTests(jsonFile string, flakyFile string) (*reviewOutput, error) {
	jf, err := os.Open(jsonFile)
	if err != nil {
		return nil, fmt.Errorf("open %s: %s", jsonFile, err)
	}
	defer jf.Close()

	var ff *os.File
	if flakyFile != "" {
		ff, err = os.Open(flakyFile)
		if err != nil {
			return nil, fmt.Errorf("open %s: %s", flakyFile, err)
		}
		defer ff.Close()
	}
	return reviewTestsReaders(jf, ff)
}

func reviewTestsReaders(jf io.Reader, ff io.Reader) (*reviewOutput, error) {
	var failedTests, flakyTests, rerunTests strings.Builder
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
		if res, ok := testResults[testKey(ev.Test, ev.Package)]; ok {
			// If the test is already recorded as passed, it means the test
			// eventually succeeded.
			if res.Action == "pass" {
				continue
			}
			if res.Action == "fail" {
				rerunTests.WriteString(fmt.Sprintf(rerunFormat, ev.Package, ev.Test, ev.Action))
			}
			// overwrite previously failed result
			if res.Action == "fail" && ev.Action == "pass" {
				testResults[testKey(ev.Test, ev.Package)] = ev
			}
		} else {
			testResults[testKey(ev.Test, ev.Package)] = ev
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("json line scan: %s", err)
	}

	for _, ev := range testResults {
		if ev.Action == "fail" {
			if kf.IsFlaky(ev.Package, ev.Test) {
				flakyTests.WriteString(fmt.Sprintf(flakyFormat, ev.Package, ev.Test))
			} else {
				failedTests.WriteString(fmt.Sprintf(failFormat, ev.Package, ev.Test))
			}
		}
	}

	return &reviewOutput{
		Failed: failedTests.String(),
		ReRuns: rerunTests.String(),
		Flaky:  flakyTests.String(),
	}, nil
}
