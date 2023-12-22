// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tests implements the unit tests for pkg/compliance.
package tests

import (
	"testing"
)

func TestBase(t *testing.T) {
	b := newTestBench(t)
	defer b.Run()

	b.AddRule("BadYamlSuiteInput").
		WithInput(`zsd`).
		WithRego(``).
		AssertError()

	b.AddRule("DuplicatedTag").
		WithInput(`
- file:
		path: plop
	tag: abc
- file:
		name: foobar
	tag: abc
`).
		AssertErrorEvent()

	b.AddRule("UnknownInput").
		WithInput(`
- asdpoakspo:
		path: plop
	tag: abc
`).
		AssertErrorEvent()

	b.AddRule("NoInput").
		WithInput("").
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	true
	f := dd.passed_finding(
		"foo",
		"bar",
		{},
	)
}
`).
		AssertError()

	b.AddRule("Constants").
		WithInput(`
- constants:
		foo: bar
		baz: baz
	tag: constant1
- constants:
		quz: 123
	tag: constant2
`).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	input.constant1.foo == "bar"
	input.constant1.baz == "baz"
	input.constant2.quz == 123
	f := dd.passed_finding(
		"foo",
		"bar",
		{},
	)
}
`).
		AssertPassedEvent(nil)

	b.AddRule("Constants2").
		WithInput(`
- constants:
		foo: bar
		quz: 1
`).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	input.constants.foo == "bar"
	input.constants.quz == 1
	f := dd.passed_finding("foo", "bar", {})
}
`).
		AssertPassedEvent(nil)

	b.AddRule("NoOutputRego").
		WithInput(`
- constants:
		foo: bar
`).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	input.nopenope
	f := dd.passed_finding("foo", "bar", {})
}
`).
		AssertNoEvent()

	b.AddRule("BadRego").
		WithInput(`
- file:
		path: plop
	tag: abcd
`).
		WithRego(`{`).
		AssertErrorEvent()
}
