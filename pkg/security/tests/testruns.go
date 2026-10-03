// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

// The suite is partitioned into groups that each build the eBPF module once.
// m.Run() cannot be reordered, so a group is a separate invocation of the test
// binary: the KMT runner (test/new-e2e/system-probe/test-runner) reads the
// names from -list-groups and runs one pass per name with -group.
//
// Membership derives from the declaration registry (testdecl.go), so
// -list-groups runs no test.

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"unsafe"
)

const (
	// groupLinePrefix marks the -list-groups lines: the suite shares stdout
	// with anything logged during init().
	groupLinePrefix = "testgroup "

	configGroupPrefix = "config-"
	inlineGroup       = "inline-config"

	// defaultGroup is a real config group, not a leftover bucket: its tests all
	// use the default config, so it builds the module once for most of the
	// suite. Its membership is negative, which is what lands a test that
	// declares nothing in a group at all.
	defaultGroup = "default"
)

// groupNames returns the groups in execution order, defaultGroup last so a
// suite with nothing declared degenerates to one pass.
func groupNames() []string {
	sigs := map[string]bool{}
	var inline bool
	for _, d := range declaredConfigs {
		switch {
		case d.inlineConfig:
			inline = true
		case d.signature != defaultSignature:
			sigs[d.signature] = true
		}
	}

	names := make([]string, 0, len(sigs)+2)
	for sig := range sigs {
		names = append(names, configGroupPrefix+sig)
	}
	slices.Sort(names)
	if inline {
		names = append(names, inlineGroup)
	}
	return append(names, defaultGroup)
}

// groupOf returns the one group a top-level entry belongs to. Being a total
// function is what makes the groups a partition: every entry lands in exactly
// one pass, whether or not it declares anything.
func groupOf(name string) string {
	d, declared := declaredConfigs[name]
	switch {
	case !declared:
		return defaultGroup
	case d.inlineConfig:
		return inlineGroup
	case d.signature == defaultSignature:
		return defaultGroup
	default:
		return configGroupPrefix + d.signature
	}
}

func printGroups() {
	for _, name := range groupNames() {
		fmt.Println(groupLinePrefix + name)
	}
}

// entryTables are the tables go test generated for this binary, the same ones
// -test.list walks. All four are trimmed: examples and fuzz targets execute in
// a plain `go test`, benchmarks under -bench.
//
// The fields are unexported, so callers must handle !ok -- a future Go could
// rename them, and running the wrong set of tests is worse than not grouping.
func entryTables(m *testing.M) ([]reflect.Value, bool) {
	v := reflect.ValueOf(m).Elem()

	var tables []reflect.Value
	for _, name := range []string{"tests", "benchmarks", "fuzzTargets", "examples"} {
		f := v.FieldByName(name)
		if !f.IsValid() || !f.CanAddr() || f.Kind() != reflect.Slice {
			return nil, false
		}
		tables = append(tables, reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem())
	}
	return tables, true
}

// selectGroup trims the test tables to group, which leaves -test.run and
// -test.skip entirely the job's. It reports false for an unknown group or for
// tables it cannot reach.
func selectGroup(m *testing.M, group string) bool {
	if !slices.Contains(groupNames(), group) {
		return false
	}
	tables, ok := entryTables(m)
	if !ok {
		return false
	}

	for _, table := range tables {
		kept := reflect.MakeSlice(table.Type(), 0, table.Len())
		for i := range table.Len() {
			if e := table.Index(i); groupOf(e.FieldByName("Name").String()) == group {
				kept = reflect.Append(kept, e)
			}
		}
		table.Set(kept)
	}
	return true
}
