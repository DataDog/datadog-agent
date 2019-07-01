package testrtloader

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
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
	ver, path := getPyInfo()
	prefix := "3."
	if common.UsingTwo {
		prefix = "2.7."
	}

	if !strings.HasPrefix(ver, prefix) {
		t.Errorf("Version doesn't start with `%s`: %s", prefix, ver)
	}

	if path == "" {
		t.Errorf("Python path is null")
	}
}

func TestRunSimpleString(t *testing.T) {
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
}

func TestGetError(t *testing.T) {
	errorStr := getError()
	expected := "unable to import module 'foo': No module named 'foo'"
	if common.UsingTwo {
		expected = "unable to import module 'foo': No module named foo"
	}
	if errorStr != expected {
		t.Fatalf("Wrong error string returned: %s", errorStr)
	}
}

func TestHasError(t *testing.T) {
	if !hasError() {
		t.Fatal("has_error should return true, got false")
	}
}

func TestGetCheck(t *testing.T) {
	version, err := getFakeCheck()

	if err != nil {
		t.Fatal(err)
	}

	if version != "0.4.2" {
		t.Fatalf("expected version '0.4.2', found '%s'", version)
	}
}

func TestRunCheck(t *testing.T) {
	res, err := runFakeCheck()

	if err != nil {
		t.Fatal(err)
	}

	if res != "" {
		t.Fatal(res)
	}
}

func TestGetCheckWarnings(t *testing.T) {
	res, err := runFakeGetWarnings()

	if err != nil {
		t.Fatal(err)
	}

	if res[0] != "warning 1" || res[1] != "warning 2" || res[2] != "warning 3" {
		t.Fatal(res)
	}
}

func TestGetIntegrationsList(t *testing.T) {
	res, err := getIntegrationList()

	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"foo", "bar", "baz"}

	if !reflect.DeepEqual(expected, res) {
		t.Fatalf("Expected %v, got %v", expected, res)
	}
}

func TestSetModuleAttrString(t *testing.T) {
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
}
