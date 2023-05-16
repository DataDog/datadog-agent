// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package tests

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"

	"github.com/stretchr/testify/assert"
)

func TestFile(t *testing.T) {
	b := NewTestBench(t)
	defer b.Run()

	b.AddRule("FileNotExists").
		WithInput(`
- file:
		name: %q
		parser: raw
	tag: thefile
`, filepath.Join(b.rootDir, "/foo/bar/baz/quz/quuz")).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	not has_key(input, "thefile")
	not has_key(input, "file")
	f := dd.failing_finding(
		"my_resource_type",
		"my_resource_id",
		{"foo": "bar"}
	)
}
`).
		AssertFailedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "my_resource_id", evt.ResourceID)
			assert.Equal(t, "my_resource_type", evt.ResourceType)
			assert.Equal(t, "bar", evt.Data["foo"])
		})

	tmpFile := b.WriteTempFile(t, "foobar")

	b.AddRule("FileExistsNoParser").
		WithInput(`
- file:
		path: %s
	tag: meh
`, tmpFile).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

compliant(f) {
	f.content == ""
	f.path == %q
	has_key(f, "group")
	has_key(f, "permissions")
	has_key(f, "user")
	has_key(f, "glob")
	f.glob == ""
}


findings[f] {
	compliant(input.meh)
	f := dd.passed_finding(
		"the_resource_type",
		"the_resource_id",
		{"content": input.meh.content}
	)
}
`, tmpFile).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "the_resource_id", evt.ResourceID)
			assert.Equal(t, "the_resource_type", evt.ResourceType)
			assert.Equal(t, "", evt.Data["content"])
		})

	b.AddRule("FileExists").
		WithInput(`
- file:
		path: %s
		parser: raw
`, tmpFile).
		WithRego(`
package datadog
import data.datadog as dd

compliant(f) {
	true
}

findings[f] {
	compliant(input.file)
	f := dd.passed_finding(
		"the_resource_type",
		"the_resource_id",
		{"content": input.file.content}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "foobar", evt.Data["content"])
		})

	b.AddRule("Context").
		WithInput(`
- file:
		path: /
- file:
		path: /foo
	tag: plop
- constants:
		foo: bar
`).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	input.context.hostname == "{{.Hostname}}"
	input.context.input.file.file.path == "/"
	input.context.input.plop.file.path == "/foo"
	input.context.input.constants.constants.foo == "bar"
	input.context.ruleID == "{{.RuleID}}"
	f := dd.passed_finding(
		"plop_type",
		"plop_id",
		{}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "plop_type", evt.ResourceType)
			assert.Equal(t, "plop_id", evt.ResourceID)
		})
}

func TestFileParser(t *testing.T) {
	b := NewTestBench(t)
	defer b.Run()

	tmpFileJSON := b.WriteTempFile(t, `{"foo":"bar","baz": {"quz": 1}}`)
	tmpFileYAML := b.WriteTempFile(t, `
foo: bar
baz:
  quz: 1
`)

	b.
		AddRule("JSONParser").
		WithInput(`
- file:
		path: %s
		parser: json
	tag: object
`, tmpFileJSON).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	input.object.content.foo == "bar"
	input.object.content.baz.quz == 1
	input.object.glob == ""
	input.object.path == %q
	_ := input.object.group
	_ := input.object.permissions
	_ := input.object.user
	f := dd.passed_finding(
		"foo",
		"bar",
		{}
	)
}
`, tmpFileJSON).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "foo", evt.ResourceType)
			assert.Equal(t, "bar", evt.ResourceID)
		})

	b.
		AddRule("YAMLParser").
		WithInput(`
- file:
		path: %s
		parser: yaml
	tag: object
`, tmpFileYAML).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	input.object.content.foo == "bar"
	input.object.content.baz.quz == 1
	input.object.glob == ""
	input.object.path == %q
	_ := input.object.group
	_ := input.object.permissions
	_ := input.object.user
	f := dd.passed_finding(
		"foo",
		"bar",
		{}
	)
}
`, tmpFileYAML).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "foo", evt.ResourceType)
			assert.Equal(t, "bar", evt.ResourceID)
		})
}

func TestFileGlob(t *testing.T) {
	b := NewTestBench(t)
	defer b.Run()

	b.
		AddRule("Glob").
		WithInput(`
- file:
		path: %s/*
		parser: raw
	type: array
	tag: files
`, b.rootDir).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	count(input.files) > 0
	f := dd.passed_finding(
		"foo",
		"bar",
		{}
	)
}
`).
		AssertPassedEvent(nil)

	b.
		AddRule("GlobEmpty").
		WithInput(`
- file:
		glob: /foo/bar/baz/quz/*
		parser: raw
	type: array
	tag: files
`).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	not has_key(input, "files")
	f := dd.passed_finding(
		"foo",
		"bar",
		{}
	)
}
`).
		AssertPassedEvent(nil)

	b.
		AddRule("Glob2").
		WithInput(`
- file:
		glob: %s/Glob2*
		parser: raw
	type: array
	tag: files
`, b.rootDir).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

valid(f) {
	has_key(f, "content")
	has_key(f, "path")
	has_key(f, "group")
	has_key(f, "permissions")
	has_key(f, "user")
	has_key(f, "glob")
}

findings[f] {
	file := input.files[_]
	valid(file)
	f := dd.passed_finding(
		"foo",
		file.path,
		{}
	)
}
`).
		AssertPassedEvent(nil).
		AssertPassedEvent(nil)
}
