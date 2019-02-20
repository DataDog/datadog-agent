package three

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
	if !strings.HasPrefix(ver, "3.") {
		t.Errorf("Version doesn't start with `3.`: %s", ver)
	}
}

func TestRunSimpleString(t *testing.T) {
	output, err := runString("print('Hello, World!', flush=True)\n")

	if err != nil {
		t.Fatalf("`run_simple_string` error: %v", err)
	}

	if output != "Hello, World!\n" {
		t.Errorf("Unexpected printed value: '%s'", output)
	}
}

func TestGetError(t *testing.T) {
	errorStr := getError()
	if errorStr != "unable to import module 'foo': No module named 'foo'" {
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
