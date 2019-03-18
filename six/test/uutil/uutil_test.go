package testuutil

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	os.Exit(m.Run())
}

func TestSubprocessOutput(t *testing.T) {
	code := fmt.Sprintf(`
	d = _util.subprocess_output(["ls"], False)
	with open(r'%s', 'w') as f:
		f.write(d)
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "/tmp" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetSubprocessOutput(t *testing.T) {
	code := fmt.Sprintf(`
	d = _util.get_subprocess_output(["ls"], False)
	with open(r'%s', 'w') as f:
		f.write(d)
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "/tmp" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
