// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"errors"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func testGroup(id, stream string) Group {
	main := config.NewEndpoint("test-key", "", "localhost", 443, "", true)
	return Group{ID: id, Stream: stream, Endpoints: config.NewEndpoints(main, nil, false, true)}
}

func TestDefinitionsLimitsAndSelections(t *testing.T) {
	groups := []Group{testGroup("a", ""), testGroup("b", "")}
	for _, limit := range []int{0, 1} {
		if _, err := NewDefinitions(groups, limit); !errors.Is(err, ErrInvalidDefinition) {
			t.Fatalf("limit %d: expected invalid definition, got %v", limit, err)
		}
	}
	if _, err := NewDefinitions(groups, 4); err != nil {
		t.Fatal(err)
	}
	five := []Group{testGroup("a", ""), testGroup("b", ""), testGroup("c", ""), testGroup("d", ""), testGroup("e", "")}
	if _, err := NewDefinitions(five, 4); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("five groups should exceed the default limit: %v", err)
	}
	if _, err := NewDefinitions(five, 5); err != nil {
		t.Fatalf("configured higher limit should accept five: %v", err)
	}
	d, _ := NewDefinitions(groups, 4)
	for _, tc := range []struct {
		selection Selection
		want      []string
		invalid   bool
	}{
		{Selection{Groups: []string{"a"}}, []string{"a"}, false},
		{Selection{Groups: []string{"a", "b", "a"}}, []string{"a", "b"}, false},
		{Selection{Excluded: true}, nil, false},
		{Selection{}, nil, true},
		{Selection{Groups: []string{"missing"}}, nil, true},
		{Selection{Excluded: true, Groups: []string{"a"}}, nil, true},
	} {
		got, err := d.Resolve(tc.selection)
		if tc.invalid != errors.Is(err, ErrInvalidSelection) || (!tc.invalid && !reflect.DeepEqual(got, tc.want)) {
			t.Fatalf("selection %+v: got %v, %v; want %v invalid=%v", tc.selection, got, err, tc.want, tc.invalid)
		}
	}
}

func TestDeclaredSharedStreamMustMatch(t *testing.T) {
	a, b := testGroup("a", "shared"), testGroup("b", "shared")
	b.Endpoints.BatchMaxSize++
	if _, err := NewDefinitions([]Group{a, b}, 4); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("incompatible stream: %v", err)
	}
	b.Endpoints.BatchMaxSize = a.Endpoints.BatchMaxSize
	d, err := NewDefinitions([]Group{a, b}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Resolve(Selection{Groups: []string{"a"}}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("partial stream: %v", err)
	}
	if groups, err := d.Resolve(Selection{Groups: []string{"a", "b"}}); err != nil || len(groups) != 2 {
		t.Fatalf("complete stream: %v, %v", groups, err)
	}
}
