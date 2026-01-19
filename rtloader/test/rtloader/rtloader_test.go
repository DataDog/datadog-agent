// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testrtloader

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	ret := m.Run()

	tearDown()

	os.Exit(ret)
}

func TestGetPyInfo(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	ver, path := getPyInfo()
	if !strings.HasPrefix(ver, "3.") {
		t.Errorf("Version doesn't start with `3.`: %s", ver)
	}

	if path == "" {
		t.Errorf("Python path is null")
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestRunSimpleString(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
with open(r'%s', 'w') as f:
	f.write('Hello, World!')`, tmpfile.Name())

	output, err := runString(code)

	if err != nil {
		t.Fatalf("`run_simple_string` error: %v", err)
	}

	if output != "Hello, World!" {
		t.Errorf("Unexpected printed value: '%s'", output)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSysExecutableValue(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
import sys
with open(r'%s', 'w') as f:
 f.write(sys.executable)`, tmpfile.Name())

	output, err := runString(code)
	if err != nil {
		t.Fatalf("`test_sys_executable` error: %v", err)
	}

	if output != "/folder/mock_python_interpeter_bin_path" {
		t.Errorf("Unexpected sys.executable value: '%s'", output)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSysHomeValue(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
import sys
with open(r'%s', 'w') as f:
 f.write(sys.prefix)`, tmpfile.Name())

	output, err := runString(code)
	if err != nil {
		t.Fatalf("`test_sys_home` error: %v", err)
	}

	// Check if the path exists and contains Python-related directories
	if output == "" {
		t.Errorf("Python home path is empty")
	}

	// Check if the path contains typical Python installation directories
	output = strings.ToLower(output)
	if !strings.Contains(output, "python") && !strings.Contains(output, "miniforge") && !strings.Contains(output, "conda") {
		t.Errorf("Python home path '%s' does not appear to be a valid Python installation directory", output)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSysUTFValue(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
import sys
with open(r'%s', 'w') as f:
 f.write(str(sys.flags.utf8_mode))`, tmpfile.Name())

	output, err := runString(code)
	if err != nil {
		t.Fatalf("`test_sys_home` error: %v", err)
	}

	if output != "1" {
		t.Errorf("UTF-8 mode is not enabled, got: %s", output)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetError(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	errorStr := getError()
	if errorStr != "unable to import module 'foo': No module named 'foo'" {
		t.Fatalf("Wrong error string returned: %s", errorStr)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestHasError(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	if !hasError() {
		t.Fatal("has_error should return true, got false")
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetCheck(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	version, err := getFakeCheck()

	if err != nil {
		t.Fatal(err)
	}

	if version != "0.4.2" {
		t.Fatalf("expected version '0.4.2', found '%s'", version)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestRunCheck(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	res, err := runFakeCheck()

	if err != nil {
		t.Fatal(err)
	}

	if res != "" {
		t.Fatal(res)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetCheckWarnings(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	res, err := runFakeGetWarnings()

	if err != nil {
		t.Fatal(err)
	}

	if res[0] != "warning 1" || res[1] != "warning 2" || res[2] != "warning 3" {
		t.Fatal(res)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetIntegrationsList(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	res, err := getIntegrationList()

	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"foo", "bar", "baz"}

	if !reflect.DeepEqual(expected, res) {
		t.Fatalf("Expected %v, got %v", expected, res)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetModuleAttrString(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	setModuleAttrString("sys", "test", "hello")

	code := fmt.Sprintf(`
import sys
with open(r'%s', 'w') as f:
	f.write(getattr(sys, "test", "attr 'test' not set"))`, tmpfile.Name())

	output, err := runString(code)

	if err != nil {
		t.Fatalf("`run_simple_string` error: %v", err)
	}

	if output != "hello" {
		t.Errorf("Unexpected printed value: '%s'", output)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestCancelCheck(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := `import fake_check
fake_check.was_canceled = False`

	if _, err := runString(code); err != nil {
		t.Fatalf("`TestCancelCheck` error resetting was_canceled: %v", err)
	}

	if err := cancelFakeCheck(); err != nil {
		t.Fatal(err)
	}

	code = fmt.Sprintf(`
import sys
import fake_check
with open(r'%s', 'w') as f:
	f.write(str(fake_check.was_canceled))`, tmpfile.Name())

	output, err := runString(code)

	if err != nil {
		t.Fatalf("`cancel_check` error: %v", err)
	}

	if output != "True" {
		t.Errorf("Unexpected printed value: '%s'", output)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetAttrBool(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	tests := []struct {
		name    string
		code    string
		success bool
		value   bool
	}{

		{
			name: "Test with a True attribute",
			code: `import fake_check
fake_check.foo = True`,
			success: true,
			value:   true,
		}, {
			name: "Test with a False attribute",
			code: `import fake_check
fake_check.foo = False`,
			success: true,
			value:   false,
		}, {
			name: "Test with a non-bool attribute",
			code: `import fake_check
fake_check.foo = "bar"`,
			success: false,
			value:   false,
		}, {
			name: "Test with a None attribute",
			code: `import fake_check
fake_check.foo = None`,
			success: false,
			value:   false,
		}, {
			name:    "Test with a non-existing attribute",
			code:    `import fake_check`,
			success: false,
			value:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := runString(tt.code); err != nil {
				t.Fatalf("`TestGetAttrBool` error running code for test case: %v", err)
			}

			res, err := getFakeModuleWithBool()

			if tt.success && err != nil {
				t.Fatal(err)
			}

			if res != tt.value {
				t.Fatalf("Expected %v, got %v", tt.value, res)
			}
		})
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}
