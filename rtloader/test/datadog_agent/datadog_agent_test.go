package testdatadogagent

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
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
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetConfig(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestHeaders(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetHostname(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetClustername(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetTracemallocEnabled(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := `assert datadog_agent.tracemalloc_enabled()`
	_, err := run(code)
	if err != nil {
		t.Fatal(err)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestLog(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetCheckMetadata(t *testing.T) {
	code := `
	datadog_agent.set_check_metadata("redis:test:12345", "version.raw", "5.0.6")
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "redis:test:12345,version.raw,5.0.6" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestSetExternalTags(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagsIgnoreNonString(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagsUnicode(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagsNotList(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagsNotTuple(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagsInvalidHostname(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagsNotDict(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagInvalidSourceType(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagInvalidTagsList(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagEmptyDict(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := `
	tags = [
		('hostname', {}),
		('hostname2', {'source_type2': ['tag3', 'tag4']}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hostname2,source_type2,tag3,tag4" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestWritePersistentCache(t *testing.T) {
	code := `
	datadog_agent.write_persistent_cache("12345", "someothervalue")
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "12345someothervalue" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestReadPersistentCache(t *testing.T) {
	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		data = datadog_agent.read_persistent_cache("12345")
		assert type(data) == type("")
		f.write(data)
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "somevalue" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
