// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package flake marks an instance of [testing.TB](https://pkg.go.dev/testing#TB) as flake.
// Use [flake.Mark] to mark a known flake test.
// Use `skip-flake` to control the behavior, or set the environment variable `SKIP_FLAKE`.
// Flags take precedence over environment variables.
package flake

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

const flakyTestMessage = "flakytest: this is a known flaky test"

var skipFlake = flag.Bool("skip-flake", false, "skip tests labeled as flakes")
var flakyPatternsConfigMutex = sync.Mutex{}

// Mark test as a known flaky.
// If any of skip-flake flag or GO_TEST_SKIP_FLAKE environment variable is set, the test will be skipped.
// Otherwise test will be marked as known flake through a special message on tests output.
func Mark(t testing.TB) {
	t.Helper()
	t.Log(flakyTestMessage)
	if shouldSkipFlake() {
		t.Skip("flakytest: skip known flaky test")
		return
	}
}

// MarkOnJobName marks the test as flaky if the CI_JOB_NAME environment variable exists and
// contains any of the job names provided. A partial match is considered a match.
func MarkOnJobName(t testing.TB, jobNames ...string) {
	t.Helper()
	jobName := os.Getenv("CI_JOB_NAME")
	if jobName == "" {
		return
	}
	for _, jobNamePartial := range jobNames {
		if strings.Contains(jobName, jobNamePartial) {
			Mark(t)
			return
		}
	}
}

// Get the test function package which is the topmost function in the stack that is part of the datadog-agent package
func getPackageName() (string, error) {
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

	if fullPackageName == "" {
		return "", errors.New("failed to fetch e2e test function information")
	}

	prefix := filepath.FromSlash("github.com/DataDog/datadog-agent/")
	fullPackageName = strings.TrimPrefix(fullPackageName, prefix)
	nameParts := strings.Split(fullPackageName, ".")
	packageName := nameParts[0]

	return packageName, nil
}

// MarkOnLog marks the test as flaky when the 'text' log is found, if you need to use regex see [MarkOnLogRegex]
func MarkOnLog(t testing.TB, text string) {
	t.Helper()
	MarkOnLogRegex(t, regexp.QuoteMeta(text))
}

// MarkOnLogRegex marks the test as flaky when the `pattern` regular expression is found in its logs.
func MarkOnLogRegex(t testing.TB, pattern string) {
	// Types for the yaml file
	type testEntry struct {
		Test  string `yaml:"test"`
		OnLog string `yaml:"on-log"`
	}
	type configEntries = map[string][]testEntry

	t.Helper()
	flakyPatternsConfig := os.Getenv("FLAKY_PATTERNS_CONFIG")
	if flakyPatternsConfig == "" {
		t.Log("Warning: flake.MarkOnLog will not mark tests as flaky since FLAKY_PATTERNS_CONFIG is not set")
		return
	}

	// Avoid race conditions
	flakyPatternsConfigMutex.Lock()
	defer flakyPatternsConfigMutex.Unlock()

	flakyConfig := make(configEntries)

	// Read initial config
	_, err := os.Stat(flakyPatternsConfig)
	if err == nil {
		f, err := os.Open(flakyPatternsConfig)
		if err != nil {
			t.Logf("Warning: failed to open flaky patterns config file: %v", err)
			return
		}
		defer f.Close()

		dec := yaml.NewDecoder(f)
		err = dec.Decode(&flakyConfig)
		if err != nil {
			t.Logf("Warning: failed to decode flaky patterns config file: %v", err)
			return
		}
	}

	packageName, err := getPackageName()
	if err != nil {
		t.Logf("Warning: failed to get package name: %v", err)
		return
	}

	// Update config by adding an entry to this test with this pattern
	entry := testEntry{Test: t.Name(), OnLog: pattern}
	if packageConfig, ok := flakyConfig[packageName]; ok {
		flakyConfig[packageName] = append(packageConfig, entry)
	} else {
		flakyConfig[packageName] = []testEntry{entry}
	}

	// Write config back
	f, err := os.OpenFile(flakyPatternsConfig, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Logf("Warning: failed to open flaky patterns config file: %v", err)
		return
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	err = encoder.Encode(flakyConfig)
	if err != nil {
		t.Logf("Warning: failed to encode flaky patterns config file: %v", err)
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
