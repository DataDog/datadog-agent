// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package nodetreemodel

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func panicInTest(format string, params ...interface{}) {
	panic(log.Errorf(format, params...))
}

func (c *ntmConfig) toDebugString(source model.Source, opts ...model.StringifyOption) (string, error) {
	strcfg := model.StringifyConfig{}
	for _, opt := range opts {
		opt(&strcfg)
	}

	// Collect the filters for settings
	var filterSet map[string]struct{}
	var traverseSet map[string]struct{}
	if strcfg.SettingFilters != nil {
		filterSet = make(map[string]struct{}, len(strcfg.SettingFilters))
		traverseSet = make(map[string]struct{})
		for _, setting := range strcfg.SettingFilters {
			filterSet[setting] = struct{}{}
			parts := strings.Split(setting, ".")
			for i := 0; i < len(parts)-1; i++ {
				partial := strings.Join(parts[:i+1], ".")
				traverseSet[partial] = struct{}{}
			}
		}
	}

	if c.td == nil {
		c.td = &treeDebugger{seenPtrs: make(map[string]*ptrCount)}
	}
	c.td.cfg = strcfg
	c.td.filterSet = filterSet
	c.td.traverseSet = traverseSet

	if source == "all" {
		lines := []string{}
		allSources := append([]model.Source{model.Source("root")}, model.Sources...)
		for _, src := range allSources {
			tree, _ := c.getTreeBySource(src)
			single, err := c.td.stringifySourceTree(tree, src)
			if err != nil {
				return "", err
			}
			lines = append(lines, single...)
		}
		lines = c.td.appendExtraDetails(lines)
		return strings.Join(lines, "\n"), nil
	}

	tree, err := c.getTreeBySource(source)
	if err != nil {
		return "", err
	}
	lines, err := c.td.stringifySourceTree(tree, source)
	if err != nil {
		return "", err
	}
	lines = c.td.appendExtraDetails(lines)
	return strings.Join(lines, "\n"), nil
}

type ptrCount struct {
	key int
	num int
}

type treeDebugger struct {
	cfg          model.StringifyConfig
	filterSet    map[string]struct{}
	traverseSet  map[string]struct{}
	seenPtrOrder []string
	seenPtrs     map[string]*ptrCount
}

func (d *treeDebugger) appendExtraDetails(lines []string) []string {
	if !d.cfg.DedupPointerAddr {
		return lines
	}

	res := []string{}
	for _, addr := range d.seenPtrOrder {
		pc := d.seenPtrs[addr]
		res = append(res, fmt.Sprintf("#ptr<%06d> (%d) => %s", pc.key, pc.num, addr))
	}
	return append(lines, res...)
}

func (d *treeDebugger) stringifySourceTree(tree Node, src model.Source) ([]string, error) {
	if tree == nil {
		return nil, fmt.Errorf("invalid source: %s", src)
	}
	if len(tree.(InnerNode).ChildrenKeys()) == 0 {
		// If a tree is empty, don't process its root nodes. This allows
		// OmitPointerAddr to correctly count unique nodes that actually appear.
		return nil, nil
	}
	res, err := d.nodeToString(tree, 0, "")
	if err != nil {
		return res, err
	}
	ptr := d.makePointer(tree)
	res = append([]string{fmt.Sprintf("tree(%s) source=%s", ptr, src)}, res...)
	return res, nil
}

func (d *treeDebugger) branchCheckFilter(path string) (bool, bool) {
	if path == "" {
		return true, true
	}
	if d.filterSet == nil {
		return true, true
	}
	parts := strings.Split(path, ".")
	for i := range parts {
		part := strings.Join(parts[:i+1], ".")
		if _, found := d.filterSet[part]; found {
			return true, true
		}
	}
	if _, found := d.traverseSet[path]; found {
		return true, false
	}
	return false, false
}

func (d *treeDebugger) nodeToString(n Node, depth int, path string) ([]string, error) {
	isAllowBranch, isPrefixSetting := d.branchCheckFilter(path)
	if !isAllowBranch {
		return []string{}, nil
	}

	padding := strings.Repeat("  ", depth)
	if n == nil {
		return []string{fmt.Sprintf("%s  %v", padding, "<nil>")}, nil
	}
	if leaf, ok := n.(LeafNode); ok {
		if !isPrefixSetting {
			return []string{}, nil
		}
		val := leaf.Get()
		source := leaf.Source()
		ptr := d.makePointer(n)
		showval := fmt.Sprintf("%v", val)
		if strval, ok := val.(string); ok {
			showval = fmt.Sprintf("%q", strval)
		}
		return []string{fmt.Sprintf("%s  leaf(%s), val:%v, source:%s", padding, ptr, showval, source)}, nil
	}
	inner, ok := n.(InnerNode)
	if !ok {
		return nil, fmt.Errorf("unknown node type: %T", n)
	}
	keys := inner.ChildrenKeys()
	ptr := d.makePointer(n)
	result := []string{}
	if depth > 0 && (isAllowBranch || isPrefixSetting) {
		result = append(result, fmt.Sprintf("%sinner(%s)", padding, ptr))
	}
	for _, key := range keys {
		nextPath := path + "." + key
		if path == "" {
			nextPath = key
		}
		msg := fmt.Sprintf("%s> %s", padding, key)
		child, _ := n.GetChild(key)
		rest, err := d.nodeToString(child, depth+1, nextPath)
		if err != nil {
			return nil, err
		}
		isAllowBranch, isPrefixSetting := d.branchCheckFilter(nextPath)
		if isAllowBranch || isPrefixSetting {
			rest = append([]string{msg}, rest...)
		}
		result = append(result, rest...)
	}
	return result, nil
}

func (d *treeDebugger) makePointer(object interface{}) string {
	ptr := fmt.Sprintf("%p", object)
	if d.cfg.DedupPointerAddr || d.cfg.OmitPointerAddr {
		if _, found := d.seenPtrs[ptr]; !found {
			d.seenPtrs[ptr] = &ptrCount{
				key: len(d.seenPtrs),
				num: 0,
			}
			d.seenPtrOrder = append(d.seenPtrOrder, ptr)
		}
		pc := d.seenPtrs[ptr]
		pc.num++
		return fmt.Sprintf("#ptr<%06d>", pc.key)
	}
	return ptr
}
