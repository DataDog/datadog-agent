package testcontainers

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
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
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestIsExcludedErrorTypeName(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		f.write("{},{}".format(
			containers.is_excluded(123, 'bar'),
		))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}

	if matched, err := regexp.Match("TypeError: argument 1 must be (str|string), not int", []byte(out)); err != nil && !matched {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestIsExcludedErrorTypeImage(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		f.write("{},{}".format(
			containers.is_excluded('foo', 123),
		))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := regexp.Match("TypeError: argument 2 must be (str|string), not int", []byte(out)); err != nil && !matched {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}
