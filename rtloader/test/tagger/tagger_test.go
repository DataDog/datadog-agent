// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testtagger

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

func TestGetTags(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.get_tags("base", False)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[\"a\", \"b\", \"c\"]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetTagsHighCard(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.get_tags("base", True)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[\"A\", \"B\", \"C\"]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetTagsUnknown(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.get_tags("default_switch", True)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetTagsErrorType(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`tagger.get_tags(1234, True)`)
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

func TestTagsLow(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.get_tags("base", tagger.LOW)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[\"a\", \"b\", \"c\"]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestTagsHigh(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.tag("base", tagger.HIGH)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[\"A\", \"B\", \"C\"]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestTagsOrchestrator(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.tag("base", tagger.ORCHESTRATOR)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[\"1\", \"2\", \"3\"]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestTagsInvalidCardinality(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.tag("default_switch", 123456)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: Invalid cardinality" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestTagsUnknown(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.tag("default_switch", tagger.LOW)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestTagsErrorType(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`tagger.tag(1234, tagger.LOW)`)
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
