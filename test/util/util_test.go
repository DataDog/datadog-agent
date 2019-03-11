package testutil

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

func TestHeaders(t *testing.T) {
	code := `
	d = datadog_agent.headers(http_host="myhost", ignore_me="snafu")
	sys.stderr.write(",".join(sorted(d.keys())))
	sys.stderr.flush()
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Accept,Content-Type,Host,User-Agent" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
