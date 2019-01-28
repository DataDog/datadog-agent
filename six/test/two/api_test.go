package two

import (
	"strings"
	"testing"
)

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

func TestExtend(t *testing.T) {
	output, err := extend()

	if err != nil {
		t.Fatal(err)
	}

	if output != "I'm extending Python!\n" {
		t.Errorf("Unexpected printed value: '%s'", output)
	}
}
