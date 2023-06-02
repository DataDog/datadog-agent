// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

func main() {
	if len(os.Args) < 2 {
		log.Fatal("json file path required")
	}
	failedTests, err := reviewTests(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if len(failedTests) > 0 {
		fmt.Fprintf(os.Stderr, color.RedString(failedTests))
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

func reviewTests(jsonFile string) (string, error) {
	var failedTests strings.Builder
	jf, err := os.Open(jsonFile)
	if err != nil {
		return "", fmt.Errorf("open %s: %s", jsonFile, err)
	}

	scanner := bufio.NewScanner(jf)
	for scanner.Scan() {
		var ev testEvent
		data := scanner.Bytes()
		if err := json.Unmarshal(data, &ev); err != nil {
			return "", fmt.Errorf("json unmarshal `%s`: %s", string(data), err)
		}
		if ev.Action == "fail" && ev.Test != "" {
			failedTests.WriteString(fmt.Sprintf("FAIL: %s %s\n", ev.Package, ev.Test))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("json line scan: %s", err)
	}
	return failedTests.String(), nil
}
