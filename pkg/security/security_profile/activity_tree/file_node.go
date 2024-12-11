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
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FileNode holds a tree representation of a list of files
type FileNode struct {
	MatchedRules   []*model.MatchedRule
	Name           string
	ImageTags      []string
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
func NewFileNode(fileEvent *model.FileEvent, event *model.Event, name string, imageTag string, generationType NodeGenerationType, reducedFilePath string, resolvers *resolvers.EBPFResolvers) *FileNode {
	// call resolver. Safeguard: the process context might be empty if from a snapshot.
	if resolvers != nil && fileEvent != nil && event.ProcessContext != nil {
		resolvers.HashResolver.ComputeHashesFromEvent(event, fileEvent)
	}

	fan := &FileNode{
		Name:           name,
		GenerationType: generationType,
		IsPattern:      strings.Contains(name, "*"),
		Children:       make(map[string]*FileNode),
	}
	if imageTag != "" {
		fan.ImageTags = []string{imageTag}
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

func (fn *FileNode) getNodeLabel() string {
	label := fn.Name
	if fn.Open != nil {
		label += " [open]"
	}
	if fn.File != nil {
		if len(fn.File.PkgName) != 0 {
			label += fmt.Sprintf("|%s:%s}", fn.File.PkgName, fn.File.PkgVersion)
		}
		// add hashes
		if len(fn.File.Hashes) > 0 {
			label += fmt.Sprintf("|%v", strings.Join(fn.File.Hashes, "|"))
		} else {
			label += fmt.Sprintf("|(%s)", fn.File.HashState)
		}
	}
	return label
}

func (fn *FileNode) enrichFromEvent(event *model.Event) {
	if event == nil {
		return
	}
	if fn.FirstSeen.IsZero() {
		fn.FirstSeen = event.ResolveEventTime()
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
func (fn *FileNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, remainingPath string, imageTag string, generationType NodeGenerationType, stats *Stats, dryRun bool, reducedPath string, resolvers *resolvers.EBPFResolvers) bool {
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
			currentFn.ImageTags, _ = AppendIfNotPresent(currentFn.ImageTags, imageTag)
			continue
		}

		// create new child
		newEntry = true
		if dryRun {
			break
		}
		if len(currentPath) <= nextParentIndex+1 {
			currentFn.Children[parent] = NewFileNode(fileEvent, event, parent, imageTag, generationType, reducedPath, resolvers)
			stats.FileNodes++
			break
		}
		newChild := NewFileNode(nil, nil, parent, imageTag, generationType, "", resolvers)
		currentFn.Children[parent] = newChild
		currentFn = newChild
		currentPath = currentPath[nextParentIndex:]
	}
	return newEntry
}

func (fn *FileNode) tagAllNodes(imageTag string) {
	fn.ImageTags, _ = AppendIfNotPresent(fn.ImageTags, imageTag)
	for _, child := range fn.Children {
		child.tagAllNodes(imageTag)
	}
}

func (fn *FileNode) evictImageTag(imageTag string) bool {
	imageTags, removed := removeImageTagFromList(fn.ImageTags, imageTag)
	if !removed {
		return false
	}
	if len(imageTags) == 0 {
		return true
	}
	fn.ImageTags = imageTags
	for filename, child := range fn.Children {
		if shouldRemoveNode := child.evictImageTag(imageTag); shouldRemoveNode {
			delete(fn.Children, filename)
		}
	}
	return false
}
