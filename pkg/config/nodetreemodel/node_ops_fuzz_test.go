// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func FuzzNodeOps(f *testing.F) {

	const MaxKeyLength = 6
	f.Add("axbxc", "a", "val", int64(1))
	f.Add("network|devices", "snmp_traps", "true", int64(42))
	f.Add("alpha", "beta.gamma", "", int64(0))

	f.Fuzz(func(t *testing.T, path1 string, path2 string, valueStr string, valueInt64 int64) {
		// We split on "x" to allow for arbitrary nesting of keys any kind of special character allowed in the keys
		key1 := strings.Split(path1, "x")
		key2 := strings.Split(path2, "x")
		for i := range key1 {
			if i > MaxKeyLength {
				break
			}
			key1[i] = strings.ToValidUTF8(key1[i], "a")
		}
		for i := range key2 {
			if i > MaxKeyLength {
				break
			}
			key2[i] = strings.ToValidUTF8(key2[i], "a")
		}

		// Start with an empty inner node and set a couple of values (of different type) via SetAt
		root := newInnerNode(nil)
		if root == nil {
			return
		}
		t.Logf("key1: %+v, key2: %+v, value: %+v, n: %+v", key1, key2, valueStr, valueInt64)
		_ = root.SetAt(key1, valueStr, model.SourceFile)
		_ = root.SetAt(key2, valueInt64, model.SourceEnvVar)

		// Exercise the tree manipulation methods
		insKey := strings.Join(key1, ".")
		root.InsertChildNode(insKey, newLeafNode(valueStr, model.SourceDefault))
		root.RemoveChild(insKey)

		// Testing tree traversal methods
		_ = root.ChildrenKeys()
		_ = root.HasChild(key1[0])
		if child, err := root.GetChild(key1[0]); err == nil {
			_, _ = child.GetChild("nonexistent")
		}

		// Clone and operate on the cloned node tree
		cloned := root.Clone()
		if cinner, ok := cloned.(InnerNode); ok {
			_ = cinner.ChildrenKeys()
		}

		_ = root.DumpSettings(func(_ model.Source) bool { return true })

		// Build a secondary tree using NewNodeTree from a map and merge it
		srcMap := map[string]interface{}{
			strings.Join(key1, "."): valueStr,
			"num":                   valueInt64,
			"nested": map[string]interface{}{
				"flag": valueStr == "true",
			},
		}

		// Test leaf methods on all type of tree sources.
		for _, source := range model.Sources {
			node, err := NewNodeTree(srcMap, source)
			if err == nil {
				if other, ok := node.(InnerNode); ok {
					_, _ = root.Merge(other)
				}
			}
			if leaf, err := NewNodeTree([]interface{}{valueStr, valueInt64}, source); err == nil {
				if l, ok := leaf.(LeafNode); ok {
					_ = l.Get()
					_ = l.Source()
					_ = l.SourceGreaterThan(model.SourceSchema)
					_, _ = l.GetChild("a")
				}
			}
		}
	})
}

// TestNodeOps is a regression test for a panic in the DumpSettings method caused by a SetAt call with a key in upper case
func TestNodeOps(_ *testing.T) {
	root := newInnerNode(nil)
	// Setting this to upper case will trigger a panic in the DumpSettings method
	key1 := []string{"A"}
	key2 := []string{"0"}
	value := "0"
	n := int64(42)
	_ = root.SetAt(key1, value, model.SourceFile)
	_ = root.SetAt(key2, n, model.SourceEnvVar)
	_ = root.DumpSettings(func(_ model.Source) bool { return true }) // panic here
}
