package two

import (
	"fmt"
	"os"
	"strings"
	"testing"
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

func TestGetVersion(t *testing.T) {
	ver := getVersion()
	if !strings.HasPrefix(ver, "2.7.") {
		t.Errorf("Version doesn't start with `2.7.`: %s", ver)
	}
}

func TestRunSimpleString(t *testing.T) {
	output, err := runString("import sys; sys.stderr.write('Hello, World!') \n")

	if err != nil {
		t.Fatal("`run_simple_string` error")
	}

	if output != "Hello, World!" {
		t.Errorf("Unexpected printed value: '%s'", output)
	}
}

func TestGetError(t *testing.T) {
	errorStr := getError()
	if errorStr != "unable to import module 'foo': No module named foo" {
		t.Fatalf("Wrong error string returned: %s", errorStr)
	}
}

func TestHasError(t *testing.T) {
	if !hasError() {
		t.Fatal("has_error should return true, got false")
	}
}

func TestGetCheckAgent(t *testing.T) {
	err := getFakeCheck()

	if err != nil {
		t.Fatal(err)
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

func TestGetCheckClassOk(t *testing.T) {
	err := getCheckClass("fake_check")

	if err != nil {
		t.Fatal(err)
	}
}

func TestGetCheckClassKo(t *testing.T) {
	err := getCheckClass("unexisting_module")

	if err == nil {
		t.Fatal("Module was expected not to exist")
	}
}
