package testcontainers

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

func TestIsExcluded(t *testing.T) {
	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		f.write("{},{}".format(
			containers.is_excluded('foo', 'bar'),
			containers.is_excluded('baz', 'bar'),
		))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "True,False" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
