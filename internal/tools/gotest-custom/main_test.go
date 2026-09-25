// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package main

import (
	"reflect"
	"testing"
)

// The invoke task builds the gotestsum command as:
//
//	--raw-command ./gotest-custom {packages} -- -test.v -mod=readonly -vet=off
//	  -timeout {timeout} -tags "..." -test.count=1 ... -args ...
//
// so go test's -timeout must be translated to the test binary's -test.timeout
// to reach the prebuilt binaries.
func TestParseArgumentsForwardsGoTestTimeout(t *testing.T) {
	args := []string{
		"./tests/containers/...", "./tests/installer/...",
		"-test.v",
		"-mod=readonly",
		"-vet=off",
		"-timeout", "1h55m0s",
		"-tags", "docker",
		"-test.count=1",
		"-test.run", "TestFoo|TestBar",
		"-args", "-osdescriptors", "ubuntu:22.04", "-flavor", "base_iot",
	}

	packages, gotestArgs, testArgs := parseArguments(args)

	expectedGotestArgs := []string{
		"-test.v",
		"-test.timeout", "1h55m0s",
		"-test.count=1",
		"-test.run", "TestFoo|TestBar",
	}
	if !reflect.DeepEqual(gotestArgs, expectedGotestArgs) {
		t.Errorf("gotestArgs mismatch\nexpected: %v\nactual:   %v", expectedGotestArgs, gotestArgs)
	}
	expectedPackages := []string{"./tests/containers/...", "./tests/installer/..."}
	if !reflect.DeepEqual(packages, expectedPackages) {
		t.Errorf("packages mismatch\nexpected: %v\nactual:   %v", expectedPackages, packages)
	}
	expectedTestArgs := []string{"-osdescriptors", "ubuntu:22.04", "-flavor", "base_iot"}
	if !reflect.DeepEqual(testArgs, expectedTestArgs) {
		t.Errorf("testArgs mismatch\nexpected: %v\nactual:   %v", expectedTestArgs, testArgs)
	}
}

func TestParseArgumentsForwardsGoTestTimeoutEqualsForm(t *testing.T) {
	packages, gotestArgs, _ := parseArguments([]string{
		"./tests/containers/...",
		"-test.count=1",
		"-timeout=30m0s",
	})

	if len(packages) != 1 || packages[0] != "./tests/containers/..." {
		t.Errorf("packages mismatch, actual: %v", packages)
	}
	expectedGotestArgs := []string{"-test.count=1", "-test.timeout=30m0s"}
	if !reflect.DeepEqual(gotestArgs, expectedGotestArgs) {
		t.Errorf("gotestArgs mismatch\nexpected: %v\nactual:   %v", expectedGotestArgs, gotestArgs)
	}
}

func TestParseArgumentsForwardsGoTestTimeoutAsFirstFlag(t *testing.T) {
	_, gotestArgs, _ := parseArguments([]string{
		"./tests/containers/...",
		"-timeout", "25m0s",
		"-test.count=1",
	})

	expectedGotestArgs := []string{"-test.timeout", "25m0s", "-test.count=1"}
	if !reflect.DeepEqual(gotestArgs, expectedGotestArgs) {
		t.Errorf("gotestArgs mismatch\nexpected: %v\nactual:   %v", expectedGotestArgs, gotestArgs)
	}
}

func TestParseArgumentsKeepsTestArgsVerbatim(t *testing.T) {
	_, _, testArgs := parseArguments([]string{
		"./tests/containers/...",
		"-test.count=1",
		"-args", "-timeout", "5m",
	})

	expectedTestArgs := []string{"-timeout", "5m"}
	if !reflect.DeepEqual(testArgs, expectedTestArgs) {
		t.Errorf("testArgs mismatch\nexpected: %v\nactual:   %v", expectedTestArgs, testArgs)
	}
}
