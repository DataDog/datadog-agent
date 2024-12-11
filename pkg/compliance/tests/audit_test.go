// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tests implements the test suite of our compliance package.
package tests

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
)

func TestAuditInput(t *testing.T) {
	cl, err := compliance.DefaultLinuxAuditProvider(context.Background())
	if err != nil {
		t.Skipf("could not create audit client: %v", err)
	}

	b := newTestBench(t).
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
