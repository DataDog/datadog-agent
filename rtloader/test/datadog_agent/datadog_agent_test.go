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
		version = datadog_agent.get_version()
		if sys.version_info.major == 2:
			assert type(version) == type(b"")
		else:
			assert type(version) == type(u"")
		f.write(version)
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
		name = datadog_agent.get_hostname()
		if sys.version_info.major == 2:
			assert type(name) == type(b"")
		else:
			assert type(name) == type(u"")
		f.write(name)
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
		name = datadog_agent.get_clustername()
		if sys.version_info.major == 2:
			assert type(name) == type(b"")
		else:
			assert type(name) == type(u"")
		f.write(name)
	`, tmpfile.Name())

	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "the-cluster" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetTracemallocEnabled(t *testing.T) {
	code := `assert datadog_agent.tracemalloc_enabled()`
	_, err := run(code)
	if err != nil {
		t.Fatal(err)
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
	if out != "hostname,source_type,tag1,tag2\nhostname2,source_type2,tag3,tag4" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagsIgnoreNonString(t *testing.T) {
	code := `
	tags = [
		('hostname', {'source_type': ['tag1', 123, 'tag2']}),
		('hostname2', {'source_type2': ['tag3', [], 'tag4']}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hostname,source_type,tag1,tag2\nhostname2,source_type2,tag3,tag4" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagsUnicode(t *testing.T) {
	code := `
	tags = [
		('hostname', {'source_type': [u'tag1', 123, u'tag2']}),
		('hostname2', {'source_type2': [u'tag3', [], u'tag4']}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hostname,source_type,tag1,tag2\nhostname2,source_type2,tag3,tag4" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagsNotList(t *testing.T) {
	code := `
	datadog_agent.set_external_tags({})
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: tags must be a list" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagsNotTuple(t *testing.T) {
	code := `
	datadog_agent.set_external_tags([{}, {}])
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: external host tags list must contain only tuples" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagsInvalidHostname(t *testing.T) {
	code := `
	tags = [
		(123, {'source_type': ['tag1', 'tag2']}),
		(456, {'source_type2': ['tag3', 'tag4']}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: hostname is not a valid string" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagsNotDict(t *testing.T) {
	code := `
	tags = [
		("hostname", ('source_type', ['tag1', 'tag2'])),
		("hostname2", ('source_type2', ['tag3', 'tag4'])),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: second elem of the host tags tuple must be a dict" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagInvalidSourceType(t *testing.T) {
	code := `
	tags = [
		('hostname', {'source_type': ['tag1', 'tag2']}),
		('hostname2', {123: ['tag3', 'tag4']}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: source_type is not a valid string" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTagInvalidTagsList(t *testing.T) {
	code := `
	tags = [
		('hostname', {'source_type': {'tag1': 'tag2'}}),
		('hostname2', {'source_type2': ['tag3', 'tag4']}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: dict value must be a list of tags" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
