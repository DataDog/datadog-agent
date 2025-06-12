// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package main

import (
	"reflect"
	"testing"
)

// Helper function to compare slices, treating nil and empty slices as equivalent
func slicesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func TestParseArguments(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantPackages   []string
		wantGotestArgs []string
		wantTestArgs   []string
	}{
		{
			name:           "basic parsing with packages and test flags",
			args:           []string{"./packageA", "./packageB", "-test.timeout", "4h", "-test.count=1", "-test.run", "TestApiSuite", "-args", "os-version", "ubuntu"},
			wantPackages:   []string{"./packageA", "./packageB"},
			wantGotestArgs: []string{"-test.timeout", "4h", "-test.count=1", "-test.run", "TestApiSuite"},
			wantTestArgs:   []string{"os-version", "ubuntu"},
		},
		{
			name:           "only packages",
			args:           []string{"./packageA", "./packageB"},
			wantPackages:   []string{"./packageA", "./packageB"},
			wantGotestArgs: []string{},
			wantTestArgs:   []string{},
		},
		{
			name:           "packages with test flags but no test args",
			args:           []string{"./packageA", "-test.timeout", "30s", "-test.v"},
			wantPackages:   []string{"./packageA"},
			wantGotestArgs: []string{"-test.timeout", "30s", "-test.v"},
			wantTestArgs:   []string{},
		},
		{
			name:           "packages with non-test flags (should be ignored)",
			args:           []string{"./packageA", "-timeout", "30s", "-test.run", "TestSuite", "-verbose"},
			wantPackages:   []string{"./packageA"},
			wantGotestArgs: []string{"-test.run", "TestSuite"},
			wantTestArgs:   []string{},
		},
		{
			name:           "test flags with equals format",
			args:           []string{"./pkg", "-test.timeout=5m", "-test.count=2", "-test.run=TestName"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.timeout=5m", "-test.count=2", "-test.run=TestName"},
			wantTestArgs:   []string{},
		},
		{
			name:           "test flags with space format",
			args:           []string{"./pkg", "-test.timeout", "5m", "-test.count", "2", "-test.run", "TestName"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.timeout", "5m", "-test.count", "2", "-test.run", "TestName"},
			wantTestArgs:   []string{},
		},
		{
			name:           "mixed test flag formats",
			args:           []string{"./pkg", "-test.timeout=5m", "-test.run", "TestName", "-test.count", "1"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.timeout=5m", "-test.run", "TestName", "-test.count", "1"},
			wantTestArgs:   []string{},
		},
		{
			name:           "only test args",
			args:           []string{"-args", "config-file", "/path/to/config", "debug"},
			wantPackages:   []string{},
			wantGotestArgs: []string{},
			wantTestArgs:   []string{"config-file", "/path/to/config", "debug"},
		},
		{
			name:           "empty args",
			args:           []string{},
			wantPackages:   []string{},
			wantGotestArgs: []string{},
			wantTestArgs:   []string{},
		},
		{
			name:           "complex scenario with quoted test name",
			args:           []string{"./test/pkg1", "./test/pkg2", "-test.timeout", "10m", "-test.run", "TestApiSuite/TestSpecificCase", "-test.v", "-args", "env", "staging", "verbose"},
			wantPackages:   []string{"./test/pkg1", "./test/pkg2"},
			wantGotestArgs: []string{"-test.timeout", "10m", "-test.run", "TestApiSuite/TestSpecificCase", "-test.v"},
			wantTestArgs:   []string{"env", "staging", "verbose"},
		},
		{
			name:           "non-test flag followed by test flag",
			args:           []string{"./pkg", "-verbose", "-test.run", "TestName", "-debug", "-test.timeout=5s"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.run", "TestName", "-test.timeout=5s"},
			wantTestArgs:   []string{},
		},
		{
			name:           "test flag that looks like it has value but doesn't",
			args:           []string{"./pkg", "-test.v", "-other-flag", "-test.run", "TestName"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.v", "-test.run", "TestName"},
			wantTestArgs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPackages, gotGotestArgs, gotTestArgs := parseArguments(tt.args)

			if !slicesEqual(gotPackages, tt.wantPackages) {
				t.Errorf("parseArguments() packages = %v, want %v", gotPackages, tt.wantPackages)
			}
			if !slicesEqual(gotGotestArgs, tt.wantGotestArgs) {
				t.Errorf("parseArguments() gotestArgs = %v, want %v", gotGotestArgs, tt.wantGotestArgs)
			}
			if !slicesEqual(gotTestArgs, tt.wantTestArgs) {
				t.Errorf("parseArguments() testArgs = %v, want %v", gotTestArgs, tt.wantTestArgs)
			}
		})
	}
}

func TestParseArgumentsEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantPackages   []string
		wantGotestArgs []string
		wantTestArgs   []string
	}{
		{
			name:           "args flag with no following arguments",
			args:           []string{"./pkg", "-test.run", "TestName", "-args"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.run", "TestName"},
			wantTestArgs:   []string{},
		},
		{
			name:           "multiple args flags (only first one counts)",
			args:           []string{"./pkg", "-test.run", "TestName", "-args", "first", "-args", "second"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.run", "TestName"},
			wantTestArgs:   []string{"first", "-args", "second"},
		},
		{
			name:           "test flag with dash in value",
			args:           []string{"./pkg", "-test.run", "Test-With-Dashes", "-test.timeout", "1h30m"},
			wantPackages:   []string{"./pkg"},
			wantGotestArgs: []string{"-test.run", "Test-With-Dashes", "-test.timeout", "1h30m"},
			wantTestArgs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPackages, gotGotestArgs, gotTestArgs := parseArguments(tt.args)

			if !slicesEqual(gotPackages, tt.wantPackages) {
				t.Errorf("parseArguments() packages = %v, want %v", gotPackages, tt.wantPackages)
			}
			if !slicesEqual(gotGotestArgs, tt.wantGotestArgs) {
				t.Errorf("parseArguments() gotestArgs = %v, want %v", gotGotestArgs, tt.wantGotestArgs)
			}
			if !slicesEqual(gotTestArgs, tt.wantTestArgs) {
				t.Errorf("parseArguments() testArgs = %v, want %v", gotTestArgs, tt.wantTestArgs)
			}
		})
	}
}
