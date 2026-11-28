// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"
	"html"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var hashStateMu sync.RWMutex

// FileInfo is a slim, profile-local subset of model.FileEvent.
type FileInfo struct {
	model.FileFields

	PathnameStr string
	BasenameStr string
	Filesystem  string

	PkgName       string
	PkgVersion    string
	PkgEpoch      int
	PkgRelease    string
	PkgSrcVersion string
	PkgSrcEpoch   int
	PkgSrcRelease string

	HashState              model.HashState
	Hashes                 []string
	unaccountedAsyncHashes bool
}

// SetHashes updates HashState and Hashes under hashStateMu so background hash completion
// does not race with concurrent tree encoders or heap-size accounting.
func (fi *FileInfo) SetHashes(state model.HashState, hashes []string, async bool) {
	if fi == nil {
		return
	}
	hashStateMu.Lock()
	fi.HashState = state
	fi.Hashes = hashes
	if async && len(hashes) > 0 {
		fi.unaccountedAsyncHashes = true
	}
	hashStateMu.Unlock()
}

// GetHashes returns HashState and Hashes safely.
func (fi *FileInfo) GetHashes() (model.HashState, []string) {
	if fi == nil {
		return model.NoHash, nil
	}
	hashStateMu.RLock()
	state, hashes := fi.HashState, fi.Hashes
	hashStateMu.RUnlock()
	return state, hashes
}

func (fi *FileInfo) accountedHashes(markAccounted bool) []string {
	if fi == nil {
		return nil
	}
	if markAccounted {
		hashStateMu.Lock()
		fi.unaccountedAsyncHashes = false
		hashes := fi.Hashes
		hashStateMu.Unlock()
		return hashes
	}
	hashStateMu.RLock()
	var hashes []string
	if !fi.unaccountedAsyncHashes {
		hashes = fi.Hashes
	}
	hashStateMu.RUnlock()
	return hashes
}

// newFileInfo builds the slim FileInfo from a model.FileEvent, keeping only the fields
// serialized into the profile or read back while building/rendering the tree.
func newFileInfo(fe *model.FileEvent) *FileInfo {
	if fe == nil {
		return nil
	}
	fi := &FileInfo{
		FileFields:    fe.FileFields,
		PathnameStr:   fe.PathnameStr,
		BasenameStr:   fe.BasenameStr,
		Filesystem:    stringInterner.Deduplicate(fe.Filesystem),
		PkgName:       stringInterner.Deduplicate(fe.PkgName),
		PkgVersion:    stringInterner.Deduplicate(fe.PkgVersion),
		PkgEpoch:      fe.PkgEpoch,
		PkgRelease:    stringInterner.Deduplicate(fe.PkgRelease),
		PkgSrcVersion: stringInterner.Deduplicate(fe.PkgSrcVersion),
		PkgSrcEpoch:   fe.PkgSrcEpoch,
		PkgSrcRelease: stringInterner.Deduplicate(fe.PkgSrcRelease),
		HashState:     fe.HashState,
		Hashes:        fe.Hashes,
	}
	fi.User = stringInterner.Deduplicate(fi.User)
	fi.Group = stringInterner.Deduplicate(fi.Group)
	return fi
}

// FileNode holds a tree representation of a list of files
type FileNode struct {
	NodeBase
	MatchedRules   []*model.MatchedRule
	Name           string
	IsPattern      bool
	File           *FileInfo
	GenerationType NodeGenerationType
	Open           *OpenNode

	Children map[string]*FileNode

	// pattern is the lazily compiled matcher of a pattern node's Name.
	pattern *compiledPattern
	// signature caches structureSignature(Name).
	signature     string
	childrenIndex *childIndex
}

// OpenNode contains the relevant fields of an Open event on which we might want to write a profiling rule
type OpenNode struct {
	model.SyscallEvent
	Flags uint32
	Mode  uint32
}

// size approximates this node's own heap footprint
func (fn *FileNode) size() int64 {
	return fn.sizeWithoutChildren() + stringMapBytes(fn.Children)
}

// sizeWithoutChildren is size minus the children map, which costs a walk of
// its keys.
func (fn *FileNode) sizeWithoutChildren() int64 {
	s := int64(unsafe.Sizeof(*fn))
	s += seenBytes(fn.NodeBase)
	s += int64(len(fn.Name))
	if fn.File != nil {
		s += fileInfoStringsBytes(fn.File)
	}
	if fn.Open != nil {
		s += int64(unsafe.Sizeof(*fn.Open))
	}
	s += sliceBackingBytes(cap(fn.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	return s
}

// NewFileNode returns a new FileActivityNode instance
func NewFileNode(fileEvent *model.FileEvent, event *model.Event, name string, isPattern bool, imageTagID uint64, generationType NodeGenerationType, reducedFilePath string, resolvers *resolvers.EBPFResolvers) *FileNode {
	if fileEvent != nil && event != nil && generationType != Snapshot {
		_ = event.FieldHandlers.ResolveFileFieldsUser(event, &fileEvent.FileFields)
		_ = event.FieldHandlers.ResolveFileFieldsGroup(event, &fileEvent.FileFields)
		_ = event.FieldHandlers.ResolveFileFieldsInUpperLayer(event, &fileEvent.FileFields)
		_ = event.FieldHandlers.ResolvePackageName(event, fileEvent)
		_ = event.FieldHandlers.ResolvePackageVersion(event, fileEvent)
		_ = event.FieldHandlers.ResolvePackageEpoch(event, fileEvent)
		_ = event.FieldHandlers.ResolvePackageRelease(event, fileEvent)
		_ = event.FieldHandlers.ResolvePackageSourceVersion(event, fileEvent)
		_ = event.FieldHandlers.ResolvePackageSourceEpoch(event, fileEvent)
		_ = event.FieldHandlers.ResolvePackageSourceRelease(event, fileEvent)
	}
	fan := &FileNode{
		Name:           name,
		GenerationType: generationType,
		IsPattern:      isPattern,
	}
	fan.NodeBase = NewNodeBase()
	if event != nil {
		fan.AppendImageTagID(imageTagID, event.ResolveEventTime())
	}
	if fileEvent != nil {
		fan.File = newFileInfo(fileEvent)
		fan.File.PathnameStr = reducedFilePath
		fan.File.BasenameStr = name
	}
	// call resolver. Safeguard: the process context might be empty if from a snapshot.
	if resolvers != nil && resolvers.HashResolver != nil && fileEvent != nil && event != nil && event.ProcessContext != nil {
		resolvers.HashResolver.ComputeHashesFromEventAsync(event, fileEvent, 0, func(state model.HashState, hashes []string, async bool) {
			fan.File.SetHashes(state, hashes, async)
		})
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
		_, hashes := fn.File.GetHashes()
		out += "<TR>"
		out += "<TD>open</TD>"
		out += "<TD>" + strconv.Itoa(len(hashes)) + " hash(es)</TD>"
		out += "<TD ALIGN=\"LEFT\">" + html.EscapeString(prefix+"/"+fn.Name) + "</TD>"
		out += "<TD>" + html.EscapeString(pkg) + "</TD>"
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
func (fn *FileNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, remainingPath string, rawRemainingPath string, imageTagID uint64, generationType NodeGenerationType, stats *Stats, dryRun bool, reducedPath string, resolvers *resolvers.EBPFResolvers) (bool, *NodeBase) {
	currentFn := fn
	currentPath := remainingPath
	currentRawPath := rawRemainingPath
	newEntry := false

	for {
		parent, nextParentIndex := ExtractFirstParent(currentPath)
		if nextParentIndex == 0 {
			if !dryRun {
				sizeBefore := currentFn.sizeWithoutChildren()
				currentFn.enrichFromEvent(event)
				if stats != nil {
					stats.SizeBytes += currentFn.sizeWithoutChildren() - sizeBefore
				}
			}
			break
		}
		rawParent, nextRawParentIndex := ExtractFirstParent(currentRawPath)

		lastComponent := len(currentPath) <= nextParentIndex+1
		child, ok := findChildWithPatternFallback(currentFn.Children, &currentFn.childrenIndex, parent, lastComponent && !fileEvent.IsDir(), stats)
		if ok {
			if child.IsPattern && child.Name != parent && stats != nil {
				stats.FilePatternLookupHits++
			}
			reducedPath = withPatternComponent(reducedPath, currentPath, parent, child.Name)
			currentFn = child
			currentPath = currentPath[nextParentIndex:]
			currentRawPath = currentRawPath[nextRawParentIndex:]
			seenBefore := seenBytes(currentFn.NodeBase)
			currentFn.AppendImageTagID(imageTagID, event.ResolveEventTime())
			if stats != nil {
				stats.SizeBytes += seenBytes(currentFn.NodeBase) - seenBefore
			}
			continue
		}

		newEntry = true
		if dryRun {
			break
		}
		if currentFn.Children == nil {
			currentFn.Children = make(map[string]*FileNode)
		}
		isPattern := reducedComponentIsPattern(parent, rawParent)
		if lastComponent {
			leafNode := NewFileNode(fileEvent, event, parent, isPattern, imageTagID, generationType, reducedPath, resolvers)
			stats.FileNodes++
			currentFn = insertChildAndMerge(currentFn.Children, &currentFn.childrenIndex, parent, leafNode, stats)
			break
		}
		newChild := NewFileNode(nil, nil, parent, isPattern, imageTagID, generationType, "", resolvers)
		currentFn = insertChildAndMerge(currentFn.Children, &currentFn.childrenIndex, parent, newChild, stats)
		reducedPath = withPatternComponent(reducedPath, currentPath, parent, currentFn.Name)
		currentPath = currentPath[nextParentIndex:]
		currentRawPath = currentRawPath[nextRawParentIndex:]
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
	fn.EvictImageTag(imageTagID)
	// Directory placeholders can have no tags of their own, or different tags
	// from their children. Always visit children before deciding to prune a node.
	var removed int64
	for filename, child := range fn.Children {
		shouldRemove, childRemoved := child.evictImageTag(imageTagID)
		if shouldRemove {
			removed += stringMapEntryBytes(fn.Children, filename)
			delete(fn.Children, filename)
			fn.childrenIndex.reset()
		}
		removed += childRemoved
	}
	if fn.SeenIsEmpty() && len(fn.Children) == 0 {
		return true, removed + fn.size()
	}
	return false, removed
}
