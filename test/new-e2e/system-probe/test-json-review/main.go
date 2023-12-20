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
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

func init() {
	color.NoColor = false
}

func main() {
	failedTests, err := reviewTests("/ci-visibility/testjson/out.json")
	if err != nil {
		log.Fatal(err)
	}
	if failedTests != "" {
		fmt.Println(color.RedString(failedTests))
	} else {
		fmt.Println(color.GreenString("All tests passed."))
		return
	}

	// We want to make sure the exit code is correctly set to
	// failed here, so that the CI job also fails.
	os.Exit(1)
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

func reviewTests(jsonFile string) (string, error) {
	var failedTests strings.Builder
	jf, err := os.Open(jsonFile)
	if err != nil {
		return "", fmt.Errorf("open %s: %s", jsonFile, err)
	}
	defer jf.Close()

	scanner := bufio.NewScanner(jf)
	testResults := make(map[string]testEvent)
	for scanner.Scan() {
		var ev testEvent
		data := scanner.Bytes()
		if err := json.Unmarshal(data, &ev); err != nil {
			return "", fmt.Errorf("json unmarshal `%s`: %s", string(data), err)
		}
		if ev.Test == "" || (ev.Action != "pass" && ev.Action != "fail") {
			continue
		}
		if res, ok := testResults[testKey(ev.Test, ev.Package)]; ok {
			// If the test is already recorded as passed, it means the test
			// eventually succeeded.
			if res.Action == "pass" {
				continue
			}
			if res.Action == "fail" {
				fmt.Printf("re-ran %s %s: %s\n", ev.Package, ev.Test, ev.Action)
			}
			if res.Action == "fail" && ev.Action == "pass" {
				testResults[testKey(ev.Test, ev.Package)] = ev
			}
		} else {
			testResults[testKey(ev.Test, ev.Package)] = ev
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("json line scan: %s", err)
	}

	for _, ev := range testResults {
		if ev.Action == "fail" {
			failedTests.WriteString(fmt.Sprintf("FAIL: %s %s\n", ev.Package, ev.Test))
		}
	}

	return failedTests.String(), nil
}
