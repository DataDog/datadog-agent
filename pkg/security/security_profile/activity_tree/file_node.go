// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FileNode holds a tree representation of a list of files
type FileNode struct {
	NodeBase
	MatchedRules   []*model.MatchedRule
	Name           string
	IsPattern      bool
	File           *model.FileEvent
	GenerationType NodeGenerationType
	Open           *OpenNode

	Children map[string]*FileNode
}

// OpenNode contains the relevant fields of an Open event on which we might want to write a profiling rule
type OpenNode struct {
	model.SyscallEvent
	Flags uint32
	Mode  uint32
}

// size approximates this node's own heap footprint
func (fn *FileNode) size() int64 {
	s := int64(unsafe.Sizeof(*fn))
	s += seenBytes(fn.NodeBase)
	s += int64(len(fn.Name))
	if fn.File != nil {
		s += fileEventStringsBytes(fn.File)
	}
	if fn.Open != nil {
		s += int64(unsafe.Sizeof(*fn.Open))
	}
	s += sliceBackingBytes(cap(fn.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	s += stringMapBytes(fn.Children)
	return s
}

// NewFileNode returns a new FileActivityNode instance
func NewFileNode(fileEvent *model.FileEvent, event *model.Event, name string, imageTagID uint64, generationType NodeGenerationType, reducedFilePath string, resolvers *resolvers.EBPFResolvers) *FileNode {
	// call resolver. Safeguard: the process context might be empty if from a snapshot.
	if resolvers != nil && fileEvent != nil && event.ProcessContext != nil {
		resolvers.HashResolver.ComputeHashesFromEvent(event, fileEvent, 0)
	}

	fan := &FileNode{
		Name:           name,
		GenerationType: generationType,
		IsPattern:      strings.Contains(name, "*"),
		Children:       make(map[string]*FileNode),
	}
	fan.NodeBase = NewNodeBase()
	if event != nil {
		fan.AppendImageTagID(imageTagID, event.ResolveEventTime())
	}
	if fileEvent != nil {
		fileEventTmp := *fileEvent
		fan.File = &fileEventTmp
		fan.File.PathnameStr = reducedFilePath
		fan.File.BasenameStr = name
	}
	fan.enrichFromEvent(event)
	return fan
}

func (fn *FileNode) getNodeLabel(prefix string) string {
	var builder strings.Builder
	if prefix == "" {
		builder.WriteString(tableHeader)
		builder.WriteString("<TR>")
		builder.WriteString("<TD>Events</TD>")
		builder.WriteString("<TD>Hash count</TD>")
		builder.WriteString("<TD>File</TD>")
		builder.WriteString("<TD>Package</TD>")
		builder.WriteString("</TR>")
	}
	builder.WriteString(fn.buildNodeRow(prefix))
	for _, child := range fn.Children {
		builder.WriteString(child.getNodeLabel(prefix + "/" + fn.Name))
	}
	if prefix == "" {
		builder.WriteString("</TABLE>>")
	}
	return builder.String()
}

func (fn *FileNode) buildNodeRow(prefix string) string {
	var out string
	if fn.Open != nil && fn.File != nil {
		var pkg string
		if len(fn.File.PkgName) != 0 {
			pkg = fn.File.PkgName + ":" + fn.File.PkgVersion
		}
		out += "<TR>"
		out += "<TD>open</TD>"
		out += "<TD>" + strconv.Itoa(len(fn.File.Hashes)) + " hash(es)</TD>"
		out += "<TD ALIGN=\"LEFT\">" + prefix + "/" + fn.Name + "</TD>"
		out += "<TD>" + pkg + "</TD>"
		out += "</TR>"
	}
	return out
}

func (fn *FileNode) enrichFromEvent(event *model.Event) {
	if event == nil {
		return
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

// InsertFileEvent inserts an event in a FileNode. Returns whether a new entry was added and
// the NodeBase of the leaf FileNode reached or created.
func (fn *FileNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, remainingPath string, imageTagID uint64, generationType NodeGenerationType, stats *Stats, dryRun bool, reducedPath string, resolvers *resolvers.EBPFResolvers) (bool, *NodeBase) {
	currentFn := fn
	currentPath := remainingPath
	newEntry := false

	for {
		parent, nextParentIndex := ExtractFirstParent(currentPath)
		if nextParentIndex == 0 {
			if !dryRun {
				currentFn.enrichFromEvent(event)
			}
			break
		}

		child, ok := currentFn.Children[parent]
		if ok {
			currentFn = child
			currentPath = currentPath[nextParentIndex:]
			currentFn.AppendImageTagID(imageTagID, event.ResolveEventTime())
			continue
		}

		newEntry = true
		if dryRun {
			break
		}
		if len(currentPath) <= nextParentIndex+1 {
			leafNode := NewFileNode(fileEvent, event, parent, imageTagID, generationType, reducedPath, resolvers)
			currentFn.Children[parent] = leafNode
			stats.FileNodes++
			stats.SizeBytes += leafNode.size()
			currentFn = leafNode
			break
		}
		newChild := NewFileNode(nil, nil, parent, imageTagID, generationType, "", resolvers)
		currentFn.Children[parent] = newChild
		stats.SizeBytes += newChild.size()
		currentFn = newChild
		currentPath = currentPath[nextParentIndex:]
	}
	return newEntry, &currentFn.NodeBase
}

func (fn *FileNode) tagAllNodes(imageTagID uint64, timestamp time.Time) {
	fn.AppendImageTagID(imageTagID, timestamp)
	for _, child := range fn.Children {
		child.tagAllNodes(imageTagID, timestamp)
	}
}

func (fn *FileNode) evictImageTag(imageTagID uint64) (bool, int64) {
	if !fn.HasImageTag(imageTagID) {
		return false, 0
	}
	if fn.EvictImageTag(imageTagID) {
		return true, fileSubtreeSizeBytes(fn)
	}
	var removed int64
	for filename, child := range fn.Children {
		shouldRemove, childRemoved := child.evictImageTag(imageTagID)
		if shouldRemove {
			delete(fn.Children, filename)
		}
		removed += childRemoved
	}
	return false, removed
}
