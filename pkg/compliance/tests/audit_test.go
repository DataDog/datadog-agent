// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package tests

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance/resources/audit"
)

func TestAuditInput(t *testing.T) {
	cl, err := audit.NewAuditClient()
	if err != nil {
		t.Skipf("could not create audit client: %v", err)
	}

	b := NewTestBench(t).
		WithAuditClient(cl)
	defer b.Run()

	b.AddRule("NonExistingPath").
		WithInput(`
- audit:
    path: /foo/bar/baz
  type: array
  tag: baz
`).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	not has_key(input, "baz")
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{ "foo": input.context.hostname }
	)
}
`).
		AssertPassedEvent(nil)
}
