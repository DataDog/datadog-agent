// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package nodetreemodel

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func (c *ntmConfig) toDebugString(source model.Source) (string, error) {
	var node Node
	switch source {
	case "root":
		node = c.root
	case model.SourceEnvVar:
		node = c.envs
	case model.SourceDefault:
		node = c.defaults
	case model.SourceFile:
		node = c.file
	default:
		return "", fmt.Errorf("invalid source: %s", source)
	}
	lines, err := debugTree(node, 0)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

func debugTree(n Node, depth int) ([]string, error) {
	padding := strings.Repeat("  ", depth)
	if n == nil {
		return []string{fmt.Sprintf("%s%v", padding, "<nil>")}, nil
	}
	if leaf, ok := n.(LeafNode); ok {
		val := leaf.Get()
		source := leaf.Source()
		return []string{fmt.Sprintf("%sval:%v, source:%s", padding, val, source)}, nil
	}
	inner, ok := n.(InnerNode)
	if !ok {
		return nil, fmt.Errorf("unknown node type: %T", n)
	}
	keys := inner.ChildrenKeys()
	result := []string{}
	for _, key := range keys {
		msg := fmt.Sprintf("%s%s", padding, key)
		child, _ := n.GetChild(key)
		rest, err := debugTree(child, depth+1)
		if err != nil {
			return nil, err
		}
		result = append(result, append([]string{msg}, rest...)...)
	}
	return result, nil
}
