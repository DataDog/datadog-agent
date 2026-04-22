// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package buildschema

import (
	"testing"
)

// nodeAt returns the nested node at the given dot-separated path inside the schema.
func nodeAt(schema map[string]interface{}, path ...string) map[string]interface{} {
	curr := schema
	for _, key := range path {
		props, ok := curr["properties"].(map[string]interface{})
		if !ok {
			return nil
		}
		next, ok := props[key].(map[string]interface{})
		if !ok {
			return nil
		}
		curr = next
	}
	return curr
}

// TestLeafNodeHasNodeTypeSettings verifies that every leaf node produced by
// addToSchema carries node_type: "setting".
func TestLeafNodeHasNodeTypeSettings(t *testing.T) {
	cases := []struct {
		name string
		call func(b *builder)
	}{
		{"bool default", func(b *builder) { b.BindEnvAndSetDefault("leaf_bool", true) }},
		{"int default", func(b *builder) { b.BindEnvAndSetDefault("leaf_int", 42) }},
		{"string default", func(b *builder) { b.BindEnvAndSetDefault("leaf_string", "hello") }},
		{"float64 default", func(b *builder) { b.BindEnvAndSetDefault("leaf_float", 3.14) }},
		{"[]string default", func(b *builder) { b.BindEnvAndSetDefault("leaf_strslice", []string{"a"}) }},
		{"nested setting", func(b *builder) { b.BindEnvAndSetDefault("section.leaf", "val") }},
		{"no-default (BindEnv only)", func(b *builder) { b.BindEnv("leaf_env", "DD_LEAF_ENV") }},
		{"SetKnown", func(b *builder) { b.SetKnown("leaf_known") }},
		{"SetDefault nil", func(b *builder) { b.SetDefault("leaf_nil_default", nil) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewSchemaBuilder("", "", nil).(*builder)
			tc.call(b)

			// Determine the expected leaf location
			var leaf map[string]interface{}
			switch tc.name {
			case "nested setting":
				leaf = nodeAt(b.Schema, "section", "leaf")
			default:
				// All other cases use a top-level key
				props := b.Schema["properties"].(map[string]interface{})
				for _, v := range props {
					leaf = v.(map[string]interface{})
					break
				}
			}

			if leaf == nil {
				t.Fatal("leaf node not found in schema")
			}

			nodeType, ok := leaf["node_type"]
			if !ok {
				t.Errorf("leaf node is missing node_type field: %v", leaf)
				return
			}
			if nodeType != "setting" {
				t.Errorf("leaf node has node_type=%q, want %q", nodeType, "setting")
			}
		})
	}
}

// TestNoDefaultNodeHasBothTodoTags verifies that a setting registered without
// a default carries both TODO:fix-no-default and TODO:fix-missing-type tags,
// so the schema linter can identify it as a known issue for both checks.
func TestNoDefaultNodeHasBothTodoTags(t *testing.T) {
	b := NewSchemaBuilder("", "", nil).(*builder)
	b.BindEnv("no_default_setting", "DD_NO_DEFAULT")

	leaf := b.Schema["properties"].(map[string]interface{})["no_default_setting"].(map[string]interface{})
	tags, _ := leaf["tags"].([]string)

	wantTags := []string{"TODO:fix-no-default", "TODO:fix-missing-type"}
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}
	for _, want := range wantTags {
		if !tagSet[want] {
			t.Errorf("noDefault node is missing tag %q; got tags: %v", want, tags)
		}
	}
}

// TestSectionNodeHasNodeTypeSection verifies that intermediate section nodes
// carry node_type: "section" (existing behaviour, regression guard).
func TestSectionNodeHasNodeTypeSection(t *testing.T) {
	b := NewSchemaBuilder("", "", nil).(*builder)
	b.BindEnvAndSetDefault("my_section.my_leaf", "val")

	section := nodeAt(b.Schema, "my_section")
	if section == nil {
		t.Fatal("section node not found")
	}
	nodeType, ok := section["node_type"]
	if !ok {
		t.Error("section node is missing node_type field")
		return
	}
	if nodeType != "section" {
		t.Errorf("section node has node_type=%q, want %q", nodeType, "section")
	}
}
