package testdatadogagent

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

	ret := m.Run()

	tearDown()
	os.Exit(ret)
}

func TestGetVersion(t *testing.T) {
	code := `
	sys.stderr.write(datadog_agent.get_version())
	sys.stderr.flush()
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "1.2.3" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetConfig(t *testing.T) {
	code := `
	d = datadog_agent.get_config("foo")
	sys.stderr.write("{}:{}:{}".format(d.get('name'), d.get('body'), d.get('time')))
	sys.stderr.flush()
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "foo:Hello:123456" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
