// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tests implements the test suite of our compliance package.
package tests

import (
	"context"
	"os"
	"path/filepath"
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

type fakeAuditClient []*compliance.FileWatchRule

func (c fakeAuditClient) GetFileWatchRules() ([]*compliance.FileWatchRule, error) {
	return c, nil
}

func (c fakeAuditClient) Close() error {
	return nil
}

func TestAuditHostRoot(t *testing.T) {
	root := makeHostRoot(t)
	if err := os.WriteFile(filepath.Join(root, "run/cmp.sock"), nil, 0o660); err != nil {
		t.Fatal(err)
	}
	cl := fakeAuditClient{{Path: "/var/run/cmp.sock"}}
	b := newTestBench(t).WithHostRoot(root).WithAuditClient(cl)
	defer b.Run()

	b.AddRule("AbsoluteLink").
		WithInput(`
- audit:
		path: /var/run/cmp.sock
	type: array
	tag: watch
`).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	count(input.watch) == 1
	input.watch[0].path == "/var/run/cmp.sock"
	f := dd.passed_finding(
		"foo",
		"bar",
		{}
	)
}
`).
		AssertPassedEvent(nil)
}
