// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testdatadogagent

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestSendLog(t *testing.T) {
	code := `
	datadog_agent.send_log("log line", "postgres:test:12345")
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "log line,postgres:test:12345" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
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

func TestSetExternalTagsShortTuple(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := `
	datadog_agent.set_external_tags([('hostname',)])
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: external host tags tuple must have at least 2 elements" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSetExternalTagsEmptyTuple(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := `
	datadog_agent.set_external_tags([()])
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: external host tags tuple must have at least 2 elements" {
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

	testCases := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name: "Test query obfuscation",
			code: fmt.Sprintf(`
	result = json.loads(datadog_agent.obfuscate_sql("select * from table where id = 1"))
	with open(r'%s', 'w') as f:
		f.write(str(result['query']))
	`, tmpfile.Name()),
			expected: "select * from table where id = ?",
		},
		{
			name: "Test metadata tables_csv",
			code: fmt.Sprintf(`
	result = json.loads(datadog_agent.obfuscate_sql("select * from table where id = 1"))
	with open(r'%s', 'w') as f:
		f.write(str(result['metadata']['tables_csv']))
	`, tmpfile.Name()),
			expected: "table",
		},
		{
			name: "Test metadata commands",
			code: fmt.Sprintf(`
	result = json.loads(datadog_agent.obfuscate_sql("select * from table where id = 1"))
	with open(r'%s', 'w') as f:
		f.write(str(result['metadata']['commands'][0]))
	`, tmpfile.Name()),
			expected: "SELECT",
		},
		{
			name: "Test metadata comments",
			code: fmt.Sprintf(`
	result = json.loads(datadog_agent.obfuscate_sql("select * from table where id = 1"))
	with open(r'%s', 'w') as f:
		f.write(str(result['metadata']['comments'][0]))
	`, tmpfile.Name()),
			expected: "-- SQL test comment",
		},
	}

	for _, test := range testCases {
		out, err := run(test.code)
		if err != nil {
			t.Fatalf("%s: err: %s", test.name, err)
		}
		if out != test.expected {
			t.Fatalf("%s: expected: '%s', found: '%s'", test.name, test.expected, out)
		}
	}

	helpers.AssertMemoryUsage(t)
}

func TestObfuscateSQLErrors(t *testing.T) {
	helpers.ResetMemoryStats()

	testCases := []struct {
		input    string
		options  string
		expected string
	}{
		{"\"\"", "{'quantize_sql_tables': True}", "result is empty"},
		{"\"\"", "None", "result is empty"},
		{"{1: 2}", "{'quantize_sql_tables': False}", "argument 1 must be str(ing)?, not dict"},
		{"None", "{}", "argument 1 must be str(ing)?, not None"},
	}

	for _, c := range testCases {
		code := fmt.Sprintf(`
	try:
		result = json.loads(datadog_agent.obfuscate_sql(%s, json.dumps(%s)))
	except Exception as e:
		with open(r'%s', 'w') as f:
			f.write(str(e))
		`, c.input, c.options, tmpfile.Name())
		out, err := run(code)
		if err != nil {
			t.Fatal(err)
		}
		matched, err := regexp.MatchString(c.expected, out)
		if err != nil {
			t.Fatal(err)
		}
		if !matched {
			t.Fatalf("expected: '%s', found: '%s'", c.expected, out)
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

func TestProcessStartTime(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	exp := processStartTime // copy the value before anything touches it

	code := fmt.Sprintf(`
	with open(r'%s', 'w') as f:
		pst = datadog_agent.get_process_start_time()
		assert type(pst) == type(0.0)
		f.write(pst.hex())
	`, tmpfile.Name())

	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}

	val, err := strconv.ParseFloat(out, 64)
	if err != nil {
		t.Fatal(err)
	}

	if val != exp {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestObfuscateMongoDBString(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	cases := []struct {
		args     string
		expected string
	}{
		{
			"'{\"find\": \"customer\"}'",
			"{\"find\": \"customer\"}",
		},
	}

	for _, testCase := range cases {
		code := fmt.Sprintf(`
	result = datadog_agent.obfuscate_mongodb_string(%s)
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

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestEmitAgentTelemetry(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	cases := []string{"counter", "histogram", "gauge"}
	for _, tc := range cases {
		code := fmt.Sprintf(`
	datadog_agent.emit_agent_telemetry("test_check", "test_metric", 1.0, "%s")
			`, tc)
		_, err := run(code)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestReportIssue(t *testing.T) {
	helpers.ResetMemoryStats()

	// Note the tab is needed on the second line because of the string interpolation in datadog_agent.go:run
	code := `
	payload = '{"id":"stub-issue","issueName":"stub-issue"}'
	datadog_agent.report_issue(check_name="stub-name", report_json=payload)
`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("expected no stderr output, got %q", out)
	}

	codeErr := `
	datadog_agent.report_issue(check_name="error-check", report_json='{}')
`
	out, err = run(codeErr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "RuntimeError") || !strings.Contains(out, "stub failure") {
		t.Fatalf("expected RuntimeError with stub failure, got %q", out)
	}

	codeClear := `
	datadog_agent.report_issue(check_name="stub-name", report_json=None)
`
	out, err = run(codeClear)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("expected no output for clear path, got %q", out)
	}

	helpers.AssertMemoryUsage(t)
}

func TestResolveIssue(t *testing.T) {
	helpers.ResetMemoryStats()

	code := `
	datadog_agent.resolve_issue(issue_id="stub-issue")
`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("expected no stderr output, got %q", out)
	}

	helpers.AssertMemoryUsage(t)
}

// runLogWithStack runs code and returns the message, stack and exception type
// received by the stack aware log callback.
func runLogWithStack(t *testing.T, code string) (msg, stack, excType string) {
	t.Helper()
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(out, logWithStackSeparator)
	if len(parts) != 3 {
		t.Fatalf("unexpected output, the stack aware callback was not used: %q", out)
	}
	return parts[0], parts[1], parts[2]
}

func TestLogWithStack(t *testing.T) {
	helpers.ResetMemoryStats()

	code := `
	def inner():
		datadog_agent.log("boom", 40)

	def outer():
		inner()

	outer()
	`
	msg, stack, excType := runLogWithStack(t, code)

	if msg != "[40]boom" {
		t.Errorf("Unexpected message: %q", msg)
	}
	if excType != "<nil>" {
		t.Errorf("No exception is being handled, got exception type %q", excType)
	}
	// Frames are listed outermost first, like a Python traceback.
	iOuter := strings.Index(stack, "in outer")
	iInner := strings.Index(stack, "in inner")
	if iOuter < 0 || iInner < 0 || iOuter > iInner {
		t.Errorf("Expected outer before inner in stack, got:\n%s", stack)
	}
	if !regexp.MustCompile(`(?m)^  File "[^"\n]+", line \d+, in inner$`).MatchString(stack) {
		t.Errorf("Frame does not follow the traceback format:\n%s", stack)
	}

	helpers.AssertMemoryUsage(t)
}

func TestLogWithStackExceptionType(t *testing.T) {
	helpers.ResetMemoryStats()

	code := `
	class CustomError(Exception):
		pass

	try:
		raise CustomError("top secret value")
	except CustomError:
		datadog_agent.log("handled", 40)
	`
	_, stack, excType := runLogWithStack(t, code)

	if excType != "__main__.CustomError" {
		t.Errorf("Unexpected exception type: %q", excType)
	}
	if strings.Contains(stack, "top secret value") {
		t.Errorf("The exception message must not be captured:\n%s", stack)
	}

	helpers.AssertMemoryUsage(t)
}

func TestLogWithStackBuiltinExceptionHasNoModule(t *testing.T) {
	code := `
	try:
		{}["k"]
	except KeyError:
		datadog_agent.log("handled", 50)
	`
	msg, _, excType := runLogWithStack(t, code)

	if msg != "[50]handled" {
		t.Errorf("Critical messages use the stack aware callback, got %q", msg)
	}
	if excType != "KeyError" {
		t.Errorf("Unexpected exception type: %q", excType)
	}
}

func TestLogWithStackLevelGating(t *testing.T) {
	// Levels other than error and critical keep using the plain callback.
	for _, level := range []int{7, 10, 20, 30, 99} {
		code := fmt.Sprintf(`datadog_agent.log("quiet", %d)`, level)
		out, err := run(code)
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("[%d]quiet", level); out != want {
			t.Errorf("Level %d: expected %q from the plain callback, got %q", level, want, out)
		}
	}
}

func TestLogWithStackTruncatesDeepStacks(t *testing.T) {
	code := `
	def recurse(n):
		if n == 0:
			datadog_agent.log("deep", 40)
		else:
			recurse(n - 1)

	recurse(200)
	`
	_, stack, _ := runLogWithStack(t, code)

	frames := regexp.MustCompile(`(?m)^  File `).FindAllString(stack, -1)
	if len(frames) != 32 {
		t.Errorf("Expected the stack to be capped at 32 frames, got %d", len(frames))
	}
	if !strings.HasSuffix(strings.TrimSpace(stack), "in recurse") {
		t.Errorf("Expected the innermost frame to be kept last, got:\n%s", stack)
	}
}

func TestLogWithStackSanitizesFrameFields(t *testing.T) {
	// A code object whose file name contains a quote and a newline must not be
	// able to inject a forged frame line.
	code := `
	src = compile("datadog_agent.log('x', 40)", 'a"b\nc', 'exec')
	exec(src, {'datadog_agent': datadog_agent})
	`
	_, stack, _ := runLogWithStack(t, code)

	// run trims the output, which removes the indentation of the first line.
	for _, line := range strings.Split(strings.TrimRight(stack, "\n"), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "File ") {
			t.Errorf("Unexpected line in stack: %q\n%s", line, stack)
		}
	}
}

func TestLogWithStackInvalidArguments(t *testing.T) {
	// Argument errors are raised to Python and never reach a callback.
	out, err := run(`datadog_agent.log(None, 40)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "TypeError") {
		t.Errorf("Expected a TypeError, got %q", out)
	}
}

// BenchmarkLogWithStack measures the cost of an error log with the stack
// captured against the same log through the plain callback (level 30).
func BenchmarkLogWithStack(b *testing.B) {
	for _, level := range []int{30, 40} {
		b.Run(fmt.Sprintf("level_%d", level), func(b *testing.B) {
			code := fmt.Sprintf(`
	def f(n):
		if n == 0:
			for _ in range(%d):
				datadog_agent.log("x", %d)
		else:
			f(n - 1)
	f(12)
`, b.N, level)
			b.ResetTimer()
			if _, err := run(code); err != nil {
				b.Fatal(err)
			}
		})
	}
}

func TestLogWithStackDisabled(t *testing.T) {
	helpers.ResetMemoryStats()
	stackEnabled.Store(false)
	defer stackEnabled.Store(true)

	// The predicate says the stack is not wanted: the plain callback is used
	// and no stack is captured or delivered.
	for _, level := range []int{40, 50} {
		out, err := run(fmt.Sprintf(`datadog_agent.log("quiet", %d)`, level))
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("[%d]quiet", level); out != want {
			t.Errorf("Level %d: expected %q from the plain callback, got %q", level, want, out)
		}
	}

	// Switching it back on takes effect without re-registering callbacks.
	stackEnabled.Store(true)
	msg, stack, _ := runLogWithStack(t, `datadog_agent.log("loud", 40)`)
	if msg != "[40]loud" || stack == "<nil>" || stack == "" {
		t.Errorf("Expected the stack to be delivered again, got %q %q", msg, stack)
	}

	helpers.AssertMemoryUsage(t)
}

func TestLogWithStackTruncationKeepsValidUTF8(t *testing.T) {
	// A function name longer than the field limit, made of two byte characters
	// and a two byte prefix so the limit falls in the middle of a character.
	code := `
	name = "ff" + "é" * 300
	ns = {'datadog_agent': datadog_agent}
	exec("def " + name + "():\n  datadog_agent.log('x', 40)\n" + name + "()", ns)
	`
	_, stack, _ := runLogWithStack(t, code)

	if !utf8.ValidString(stack) {
		t.Errorf("Truncated stack is not valid UTF-8: %q", stack)
	}
	if !strings.Contains(stack, "é") {
		t.Errorf("Expected the long name to be kept, truncated, got:\n%s", stack)
	}
}
