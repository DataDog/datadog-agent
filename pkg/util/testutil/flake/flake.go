// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flake marks an instance of [testing.TB](https://pkg.go.dev/testing#TB) as flake.
// Use [flake.Mark] to mark a known flake test.
// Use `skip-flake` to control the behavior, or set the environment variable `SKIP_FLAKE`.
// Flags take precedence over environment variables.
package flake

import (
	"flag"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const flakyTestMessage = "flakytest: this is a known flaky test"

var skipFlake = flag.Bool("skip-flake", false, "skip tests labeled as flakes")
var flakyPatternsConfig = flag.String("flaky-patterns-config", "/tmp/e2e-flaky-patterns.yaml", "path to the flaky patterns configuration file that will be created when MarkOnLog is used")

// Mark test as a known flaky.
// If any of skip-flake flag or GO_TEST_SKIP_FLAKE environment variable is set, the test will be skipped.
// Otherwise test will be marked as known flake through a special message on tests output.
func Mark(t testing.TB) {
	t.Helper()
	if shouldSkipFlake() {
		t.Skip("flakytest: skip known flaky test")
		return
	}
	t.Log(flakyTestMessage)
}

// func readFlakyPatternsConfig(path string) {

// r, err := os.Open(*flakyPatternsConfig)
// if err != nil {
// 	t.Fatalf("failed to open flaky patterns config file: %v", err)
// 	return
// }
// dec := yaml.NewDecoder(r)
// // package -> []test -> {test: name, on-log: pattern}
// pkgToTests := make(map[string][]map[string]interface{})
// if err := dec.Decode(&pkgToTests); err != nil {
// 	t.Errorf("unmarshal: %w", err)
// 	return
// }
// println("Parsed")
// }

// func writeFlakyPatternsConfig(path string) {

// MarkOnLog marks the test as flaky when the `pattern` regular expression is found in its logs.
func MarkOnLog(t testing.TB, pattern string) {
	t.Helper()
	if *flakyPatternsConfig == "" {
		t.Fatal("flaky-patterns-config flag is required when using MarkOnLog")
		return
	}

	// TODO: Lock

	// Add the pattern to the yaml config

	// TODO: Lock file (multithread)
	// TODO: Not created case
	f, err := os.OpenFile(*flakyPatternsConfig, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		t.Fatalf("failed to open flaky patterns config file: %v", err)
		return
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	flakyConfig := make(map[string]interface{})
	err = dec.Decode(&flakyConfig)
	if err != nil {
		t.Fatalf("failed to decode flaky patterns config file: %v", err)
		return
	}

	// Get the test function package which is the topmost function in the stack that is part of the datadog-agent package
	fullPackageName := ""
	for i := 0; i < 42; i++ {
		pc, _, _, ok := runtime.Caller(i)
		if !ok {
			// Top of the stack
			break
		}
		fullname := runtime.FuncForPC(pc).Name()
		if strings.Contains(fullname, "datadog-agent") {
			fullPackageName = fullname
		}
	}

	// TODO A: Is it the same with subtests?
	if fullPackageName == "" {
		t.Fatalf("failed to fetch e2e test function information")
		return
	}

	// println("Caller: " + e2eCaller)

	// // TODO: How to get it properly ? If MarkOnLog is used in a subfunction, it doesn't work
	// // Get package name
	// pc, _, _, ok := runtime.Caller(1)
	// if !ok {
	// 	t.Fatalf("failed to get caller information")
	// 	return
	// }
	// fullname := runtime.FuncForPC(pc).Name()
	// TODO: Windows
	prefix := "github.com/DataDog/datadog-agent/"
	fullPackageName = strings.TrimPrefix(fullPackageName, prefix)
	nameParts := strings.Split(fullPackageName, ".")
	packageName := nameParts[0]
	println(packageName)
	println(t.Name())

	entry := make(map[string]interface{})
	entry["test"] = t.Name()
	entry["on-log"] = pattern
	if packageConfig, ok := flakyConfig[packageName]; ok {
		flakyConfig[packageName] = append(packageConfig.([]map[string]interface{}), entry)
	} else {
		flakyConfig[packageName] = []map[string]interface{}{entry}
	}

	_, err = f.Seek(0, 0)
	if err != nil {
		t.Fatalf("failed to seek flaky patterns config file: %v", err)
		return
	}
	encoder := yaml.NewEncoder(f)
	err = encoder.Encode(flakyConfig)
	if err != nil {
		t.Fatalf("failed to encode flaky patterns config file: %v", err)
		return
	}
}

func shouldSkipFlake() bool {
	if *skipFlake {
		return true
	}
	shouldSkipFlakeVar, err := strconv.ParseBool(os.Getenv("GO_TEST_SKIP_FLAKE"))
	if err != nil {
		return false
	}
	return shouldSkipFlakeVar
}
