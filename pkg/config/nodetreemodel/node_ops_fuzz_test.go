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

	f.Fuzz(func(_ *testing.T, path1 string, path2 string, valueStr string, valueInt64 int64) {
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

		// Start with an empty inner node and set a couple of values (of different type) via setAt
		root := newInnerNode(nil)
		if root == nil {
			return
		}
		_ = root.setAt(key1, valueStr, model.SourceFile)
		_ = root.setAt(key2, valueInt64, model.SourceEnvVar)

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

		_ = root.dumpSettings(true)

		// Build a secondary tree using newNodeTree from a map and merge it
		srcMap := map[string]interface{}{
			strings.Join(key1, "."): valueStr,
			"num":                   valueInt64,
			"nested": map[string]interface{}{
				"flag": valueStr == "true",
			},
		}

		// Test leaf methods on all type of tree sources.
		for _, source := range model.Sources {
			node, err := newNodeTree(srcMap, source)
			if err == nil {
				_, _ = root.Merge(node)
			}
			if leaf, err := newNodeTree([]interface{}{valueStr, valueInt64}, source); err == nil {
				_ = leaf.Get()
				_ = leaf.Source()
				_ = leaf.SourceGreaterThan(model.SourceSchema)
				_, _ = leaf.GetChild("a")
			}
		}
	})
}
