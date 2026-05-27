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

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils/pathutils"
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

// Matches returns the unified path and true if the file event used to generate the file node matches the provided
// model.FileEvent. When normalize is true, paths are compared via PathPatternBuilder so that paths differing only in
// numeric or rotated suffixes (e.g. /var/log/syslog.1 vs /var/log/syslog.2) are unified into a wildcard pattern; the
// path is required to carry an extension, to avoid collapsing unrelated files. When normalize is false, the literal
// path is returned only when both sides are byte-equal.
func (fn *FileNode) Matches(entry *model.FileEvent, normalize bool) (string, bool) {
	if fn.File == nil || entry == nil {
		return "", false
	}
	if normalize {
		var (
			nodeCommonCharsRequired = 3
			extensionRequired       = true
		)

		// relax a bit for /tmp files
		if strings.HasPrefix(fn.File.PathnameStr, "/tmp") {
			nodeCommonCharsRequired = 5
			extensionRequired = false
		}

		return pathutils.PathPatternBuilder(fn.File.PathnameStr, entry.PathnameStr, pathutils.PathPatternMatchOpts{
			WildcardLimit:           3,
			PrefixNodeRequired:      1,
			NodeSizeLimit:           8,
			NodeCommonCharsRequired: nodeCommonCharsRequired,
			ExtensionRequired:       extensionRequired,
		})
	}
	if fn.File.PathnameStr == entry.PathnameStr {
		return fn.File.PathnameStr, true
	}
	return "", false
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

// InsertFileEvent inserts an event in a FileNode. This function returns true if a new entry was added, false if
// the event was dropped.
func (fn *FileNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, remainingPath string, imageTagID uint64, generationType NodeGenerationType, stats *Stats, dryRun bool, reducedPath string, resolvers *resolvers.EBPFResolvers) bool {
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

		// create new child
		newEntry = true
		if len(currentPath) <= nextParentIndex+1 {
			// leaf: look for an existing sibling that matches the new file event by pattern
			// before creating a fresh node
			for _, sibling := range currentFn.Children {
				if pattern, ok := sibling.Matches(fileEvent, true); ok {
					newEntry = false
					if !dryRun {
						sibling.File.PathnameStr = pattern
						sibling.IsPattern = strings.Contains(pattern, "*")
						sibling.enrichFromEvent(event)
						sibling.AppendImageTagID(imageTagID, event.ResolveEventTime())
					}
					break
				}
			}
			if newEntry && !dryRun {
				currentFn.Children[parent] = NewFileNode(fileEvent, event, parent, imageTagID, generationType, reducedPath, resolvers)
				stats.FileNodes++
			}
			break
		}
		if dryRun {
			break
		}
		newChild := NewFileNode(nil, nil, parent, imageTagID, generationType, "", resolvers)
		currentFn.Children[parent] = newChild
		currentFn = newChild
		currentPath = currentPath[nextParentIndex:]
	}
	return newEntry
}

func (fn *FileNode) tagAllNodes(imageTagID uint64, timestamp time.Time) {
	fn.AppendImageTagID(imageTagID, timestamp)
	for _, child := range fn.Children {
		child.tagAllNodes(imageTagID, timestamp)
	}
}

func (fn *FileNode) evictImageTag(imageTagID uint64) bool {
	if !fn.HasImageTag(imageTagID) {
		return false
	}
	evicted := fn.EvictImageTag(imageTagID)
	if evicted {
		return true
	}
	for filename, child := range fn.Children {
		if shouldRemoveNode := child.evictImageTag(imageTagID); shouldRemoveNode {
			delete(fn.Children, filename)
		}
	}
	return false
}
