package testdatadogagent

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
		('hostname', {}),
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

func TestObfuscateSql(t *testing.T) {
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	result = datadog_agent.obfuscate_sql("select * from table where id = 1")
	with open(r'%s', 'w') as f:
		f.write(str(result))
	`, tmpfile.Name())

	out, err := run(code)

	if err != nil {
		t.Fatal(err)
	}
	expected := "select * from table where id = ?"
	if out != expected {
		t.Fatalf("expected: '%s', found: '%s'", expected, out)
	}

	helpers.AssertMemoryUsage(t)
}

func TestObfuscateSQLErrors(t *testing.T) {
	helpers.ResetMemoryStats()

	testCases := []struct {
		input    string
		expected string
	}{
		{"\"\"", "result is empty"},
		{"{1: 2}", "argument 1 must be str(ing)?, not dict"},
		{"None", "argument 1 must be str(ing)?, not None"},
	}

	for _, c := range testCases {
		code := fmt.Sprintf(`
	try:
		result = datadog_agent.obfuscate_sql(%s)
	except Exception as e:
		with open(r'%s', 'w') as f:
			f.write(str(e))
		`, c.input, tmpfile.Name())
		out, err := run(code)
		if err != nil {
			t.Fatal(err)
		}
		matched, err := regexp.MatchString(c.expected, out)
		if err != nil {
			t.Fatal(err)
		}
		if !matched {
			t.Fatalf("expected: '%s', found: '%s'", out, c.expected)
		}
	}

	helpers.AssertMemoryUsage(t)
}

func TestObfuscateSqlExecPlan(t *testing.T) {
	helpers.ResetMemoryStats()

	cases := []struct {
		args     string
		expected string
	}{
		{
			"'raw-json-plan'",
			"obfuscated",
		},
		{
			"'raw-json-plan', normalize=True",
			"obfuscated-and-normalized",
		},
		// normalize must be exactly True, anything else is evaluated to false
		{
			"'raw-json-plan', normalize=5",
			"obfuscated",
		},
	}

	for _, testCase := range cases {
		code := fmt.Sprintf(`
	result = datadog_agent.obfuscate_sql_exec_plan(%s)
	with open(r'%s', 'w') as f:
		f.write(str(result))
	`, testCase.args, tmpfile.Name())
		out, err := run(code)
		if err != nil {
			t.Fatal(err)
		}
		if out != testCase.expected {
			t.Fatalf("args: (%s) expected: '%s', found: '%s'", testCase.args, testCase.expected, out)
		}
	}

	helpers.AssertMemoryUsage(t)
}

func TestObfuscateSqlExecPlanErrors(t *testing.T) {
	helpers.ResetMemoryStats()

	cases := []struct {
		args     string
		expected string
	}{
		{
			"''",
			"empty",
		},
		{
			"{}",
			"argument 1 must be str(ing)?, not dict",
		},
	}

	for _, testCase := range cases {
		code := fmt.Sprintf(`
	try:
		result = datadog_agent.obfuscate_sql_exec_plan(%s)
	except Exception as e:
		with open(r'%s', 'w') as f:
			f.write(str(e))
	`, testCase.args, tmpfile.Name())
		out, err := run(code)
		if err != nil {
			t.Fatal(err)
		}
		matched, err := regexp.MatchString(testCase.expected, out)
		if err != nil {
			t.Fatal(err)
		}
		if !matched {
			t.Fatalf("args: (%s) expected-pattern: '%s', found: '%s'", testCase.args, testCase.expected, out)
		}
	}

	helpers.AssertMemoryUsage(t)
}
