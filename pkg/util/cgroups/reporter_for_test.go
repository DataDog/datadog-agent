// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

var tr *testReporter

type testReporter struct {
	fileAccessed map[string]int
	errors       []error
}

func init() {
	tr = newTestReporter()
	SetReporter(tr)
}

func newTestReporter() *testReporter {
	return &testReporter{
		fileAccessed: make(map[string]int),
	}
}

func (r *testReporter) reset() {
	r.fileAccessed = make(map[string]int)
	r.errors = make([]error, 0)
}

func (r *testReporter) HandleError(e error) {
	r.errors = append(r.errors, e)
}

func (r *testReporter) FileAccessed(path string) {
	hits := r.fileAccessed[path]
	r.fileAccessed[path] = hits + 1
}
