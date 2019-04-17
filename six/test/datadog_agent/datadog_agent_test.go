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
	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		f.write(datadog_agent.get_version())
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "1.2.3" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetConfig(t *testing.T) {
	code := fmt.Sprintf(`
	d = datadog_agent.get_config("foo")
	with open(r'%s', 'w') as f:
		f.write("{}:{}:{}".format(d.get('name'), d.get('body'), d.get('time')))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "foo:Hello:123456" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestHeaders(t *testing.T) {
	code := fmt.Sprintf(`
	d = datadog_agent.headers(http_host="myhost", ignore_me="snafu")
	with open(r'%s', 'w') as f:
		f.write(",".join(sorted(d.keys())))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Accept,Content-Type,Host,User-Agent" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetHostname(t *testing.T) {
	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		f.write(datadog_agent.get_hostname())
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "localfoobar" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetClustername(t *testing.T) {
	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		f.write(datadog_agent.get_clustername())
	`, tmpfile.Name())

	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "the-cluster" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestLog(t *testing.T) {
	code := `
	datadog_agent.log("foo message", 99)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[99]foo message" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTags(t *testing.T) {
	code := `
	tags = [
		('hostname', {'source_type': ['tag1', 'tag2']}),
		('hostname2', {'source_type2': ['tag3', 'tag4']}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hostname,source_type,tag1,tag2\nhostname2,source_type2,tag3,tag4\n" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
