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
