// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activity_tree

import (
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// PathsReducer is used to reduce the paths in an activity tree according to predefined heuristics
type PathsReducer struct {
	patterns        *regexp.Regexp
	callbackFuncs   map[int]func(ctx *callbackContext)
	callbackIndexes []int
}

// PatternReducer is used to reduce the paths in an activity tree according to a given pattern
type PatternReducer struct {
	Pattern            string
	GroupIndexCallback int
	GroupCount         int
	Callback           func(ctx *callbackContext)
}

// callbackContext is the input struct for the callback function
type callbackContext struct {
	start       int
	end         int
	path        string
	fileEvent   *model.FileEvent
	processNode *ProcessNode
}

// NewPathsReducer returns a new PathsReducer
func NewPathsReducer() *PathsReducer {
	patterns := getPathsReducerPatterns()
	patternsCount := len(patterns)

	r := &PathsReducer{
		callbackFuncs:   make(map[int]func(ctx *callbackContext), patternsCount),
		callbackIndexes: make([]int, patternsCount),
	}

	var fullPattern string
	var groupCount int
	for i, pattern := range patterns {
		if i > 0 {
			fullPattern += "|"
		}
		fullPattern += pattern.Pattern

		r.callbackFuncs[groupCount+pattern.GroupIndexCallback] = pattern.Callback
		r.callbackIndexes[patternsCount-1-i] = pattern.GroupIndexCallback + groupCount
		groupCount += pattern.GroupCount
	}

	r.patterns = regexp.MustCompile(fullPattern)
	return r
}

// ReducePath reduces a path according to the predefined heuristics
func (r *PathsReducer) ReducePath(path string, fileEvent *model.FileEvent, node *ProcessNode) string {
	ctx := &callbackContext{
		path:        path,
		fileEvent:   fileEvent,
		processNode: node,
	}

	allMatches := r.patterns.FindAllStringSubmatchIndex(ctx.path, -1)
	for matchSet := len(allMatches) - 1; matchSet >= 0; matchSet-- {
		matches := allMatches[matchSet]
		for _, i := range r.callbackIndexes {
			if r.callbackFuncs[i] != nil && matches[2*i] != -1 && matches[2*i+1] != -1 {
				ctx.start = matches[2*i]
				ctx.end = matches[2*i+1]
				r.callbackFuncs[i](ctx)
			}
		}
	}
	return ctx.path
}

// getPathsReducerPatterns returns the patterns used to reduce the paths in an activity tree
func getPathsReducerPatterns() []PatternReducer {
	return []PatternReducer{
		{
			Pattern:            "(/proc/(\\d+))", // process PID
			GroupIndexCallback: 2,
			GroupCount:         2,
			Callback: func(ctx *callbackContext) {
				// compute pid from path
				pid, err := strconv.Atoi(ctx.path[ctx.start:ctx.end])
				if err != nil {
					return
				}
				// replace the pid in the path between start and end with a * only if the replaced pid is not the pid of the process node
				if ctx.processNode.Process.Pid == uint32(pid) {
					ctx.path = ctx.path[:ctx.start] + "self" + ctx.path[ctx.end:]
				} else {
					ctx.path = ctx.path[:ctx.start] + "*" + ctx.path[ctx.end:]
				}
			},
		},
		{
			Pattern:            "(/task/(\\d+))", // process TID
			GroupIndexCallback: 2,
			GroupCount:         2,
			Callback: func(ctx *callbackContext) {
				ctx.path = ctx.path[:ctx.start] + "*" + ctx.path[ctx.end:]
			},
		},
		{
			Pattern:            "((kubepods-|cri-containerd-)([^/]*)\\.(slice|scope))", // kubernetes cgroup
			GroupIndexCallback: 3,
			GroupCount:         4,
			Callback: func(ctx *callbackContext) {
				if ctx.fileEvent.Filesystem == "sysfs" {
					ctx.path = ctx.path[:ctx.start] + "*" + ctx.path[ctx.end:]
				}
			},
		},
		{
			Pattern:            model.ContainerIDPatternStr, // container ID
			GroupIndexCallback: 1,
			GroupCount:         1,
			Callback: func(ctx *callbackContext) {
				ctx.path = ctx.path[:ctx.start] + "*" + ctx.path[ctx.end:]
			},
		},
		{
			Pattern:            "(/sys/devices/virtual/block/(dm-|loop)([0-9]+))", // block devices
			GroupIndexCallback: 3,
			GroupCount:         3,
			Callback: func(ctx *callbackContext) {
				if ctx.fileEvent.Filesystem == "sysfs" {
					ctx.path = ctx.path[:ctx.start] + "*" + ctx.path[ctx.end:]
				}
			},
		},
		{
			Pattern:            "(secrets/kubernetes.io/serviceaccount/([0-9._]+))", // service account token date
			GroupIndexCallback: 2,
			GroupCount:         2,
			Callback: func(ctx *callbackContext) {
				ctx.path = ctx.path[:ctx.start] + "*" + ctx.path[ctx.end:]
			},
		},
	}
}
