// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package activity_tree

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// FileNode holds a tree representation of a list of files
type FileNode struct {
	MatchedRules   []*model.MatchedRule
	Name           string
	IsPattern      bool
	File           *model.FileEvent
	GenerationType NodeGenerationType
	FirstSeen      time.Time

	Open *OpenNode

	Children map[string]*FileNode
}

// OpenNode contains the relevant fields of an Open event on which we might want to write a profiling rule
type OpenNode struct {
	model.SyscallEvent
	Flags uint32
	Mode  uint32
}

// NewFileNode returns a new FileActivityNode instance
func NewFileNode(fileEvent *model.FileEvent, event *model.Event, name string, generationType NodeGenerationType) *FileNode {
	fan := &FileNode{
		Name:           name,
		GenerationType: generationType,
		Children:       make(map[string]*FileNode),
	}
	if fileEvent != nil {
		fileEventTmp := *fileEvent
		fan.File = &fileEventTmp
	}
	fan.enrichFromEvent(event)
	return fan
}

func (fn *FileNode) getNodeLabel() string {
	label := fn.Name
	if fn.Open != nil {
		label += " [open]"
	}
	if fn.File != nil {
		if len(fn.File.PkgName) != 0 {
			label += fmt.Sprintf(" \\{%s %s\\}", fn.File.PkgName, fn.File.PkgVersion)
		}
	}
	return label
}

func (fn *FileNode) enrichFromEvent(event *model.Event) {
	if event == nil {
		return
	}
	if fn.FirstSeen.IsZero() {
		fn.FirstSeen = event.FieldHandlers.ResolveEventTimestamp(event)
	}

	fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, event.Rules)

	switch event.GetEventType() {
	case model.FileOpenEventType:
		if fn.Open == nil {
			fn.Open = &OpenNode{
				SyscallEvent: event.Open.SyscallEvent,
				Flags:        event.Open.Flags,
				Mode:         event.Open.Mode,
			}
		} else {
			fn.Open.Flags |= event.Open.Flags
			fn.Open.Mode |= event.Open.Mode
		}
	}
}

// nolint: unused
func (fn *FileNode) debug(w io.Writer, prefix string) {
	fmt.Fprintf(w, "%s %s\n", prefix, fn.Name)

	sortedChildren := make([]*FileNode, 0, len(fn.Children))
	for _, f := range fn.Children {
		sortedChildren = append(sortedChildren, f)
	}
	sort.Slice(sortedChildren, func(i, j int) bool {
		return sortedChildren[i].Name < sortedChildren[j].Name
	})

	for _, child := range sortedChildren {
		child.debug(w, "    "+prefix)
	}
}

// InsertFileEvent inserts an event in a FileNode. This function returns true if a new entry was added, false if
// the event was dropped.
func (fn *FileNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, remainingPath string, generationType NodeGenerationType, stats *ActivityTreeStats, shouldMergePaths bool, shadowInsertion bool) bool {
	currentFn := fn
	currentPath := remainingPath
	newEntry := false

	for {
		parent, nextParentIndex := ExtractFirstParent(currentPath)
		if nextParentIndex == 0 {
			if !shadowInsertion {
				currentFn.enrichFromEvent(event)
			}
			break
		}

		if shouldMergePaths && len(currentFn.Children) >= 10 && !shadowInsertion {
			currentFn.Children = fn.combineChildren(currentFn.Children, stats)
		}

		child, ok := currentFn.Children[parent]
		if ok {
			currentFn = child
			currentPath = currentPath[nextParentIndex:]
			continue
		}

		// create new child
		newEntry = true
		if len(currentPath) <= nextParentIndex+1 {
			if !shadowInsertion {
				currentFn.Children[parent] = NewFileNode(fileEvent, event, parent, generationType)
				stats.fileNodes++
			}
			break
		} else {
			newChild := NewFileNode(nil, nil, parent, generationType)
			if !shadowInsertion {
				currentFn.Children[parent] = newChild
			}

			currentFn = newChild
			currentPath = currentPath[nextParentIndex:]
			continue
		}
	}
	return newEntry
}

func (fn *FileNode) combineChildren(children map[string]*FileNode, stats *ActivityTreeStats) map[string]*FileNode {
	if len(children) == 0 {
		return children
	}

	type inner struct {
		pair utils.StringPair
		fan  *FileNode
	}

	inputs := make([]inner, 0, len(children))
	for k, v := range children {
		inputs = append(inputs, inner{
			pair: utils.NewStringPair(k),
			fan:  v,
		})
	}

	current := []inner{inputs[0]}

	for _, a := range inputs[1:] {
		next := make([]inner, 0, len(current))
		shouldAppend := true
		for _, b := range current {
			if !areCompatibleFans(a.fan, b.fan) {
				next = append(next, b)
				continue
			}

			sp, similar := utils.BuildGlob(a.pair, b.pair, 4)
			if similar {
				spGlob, _ := sp.ToGlob()
				merged, ok := mergeFans(spGlob, a.fan, b.fan)
				if !ok {
					next = append(next, b)
					continue
				}

				if stats.fileNodes > 0 { // should not happen, but just to be sure
					stats.fileNodes--
				}
				next = append(next, inner{
					pair: sp,
					fan:  merged,
				})
				shouldAppend = false
			}
		}

		if shouldAppend {
			next = append(next, a)
		}
		current = next
	}

	mergeCount := len(inputs) - len(current)
	stats.pathMergedCount.Add(uint64(mergeCount))

	res := make(map[string]*FileNode)
	for _, n := range current {
		glob, isPattern := n.pair.ToGlob()
		n.fan.Name = glob
		n.fan.IsPattern = isPattern
		res[glob] = n.fan
	}

	return res
}

func areCompatibleFans(a *FileNode, b *FileNode) bool {
	return reflect.DeepEqual(a.Open, b.Open)
}

func mergeFans(name string, a *FileNode, b *FileNode) (*FileNode, bool) {
	newChildren := make(map[string]*FileNode)
	for k, v := range a.Children {
		newChildren[k] = v
	}
	for k, v := range b.Children {
		if _, present := newChildren[k]; present {
			return nil, false
		}
		newChildren[k] = v
	}

	return &FileNode{
		Name:           name,
		File:           a.File,
		GenerationType: a.GenerationType,
		FirstSeen:      a.FirstSeen,
		Open:           a.Open, // if the 2 fans are compatible, a.Open should be equal to b.Open
		Children:       newChildren,
		MatchedRules:   model.AppendMatchedRule(a.MatchedRules, b.MatchedRules),
	}, true
}
