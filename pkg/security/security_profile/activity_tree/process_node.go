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

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"golang.org/x/exp/slices"
)

// ProcessNodeParent is an interface used to identify the parent of a process node
type ProcessNodeParent interface {
	GetParent() ProcessNodeParent
	GetChildren() *[]*ProcessNode
	GetSiblings() *[]*ProcessNode
	AppendChild(node *ProcessNode)
	AppendImageTag(imageTag string)
}

// ProcessNode holds the activity of a process
type ProcessNode struct {
	Process        model.Process
	Parent         ProcessNodeParent
	GenerationType NodeGenerationType
	ImageTags      []string
	MatchedRules   []*model.MatchedRule

	Files    map[string]*FileNode
	DNSNames map[string]*DNSNode
	Sockets  []*SocketNode
	Syscalls []int
	Children []*ProcessNode
}

// NewProcessNode returns a new ProcessNode instance
func NewProcessNode(entry *model.ProcessCacheEntry, generationType NodeGenerationType, resolvers *resolvers.EBPFResolvers) *ProcessNode {
	// call the callback to resolve additional fields before copying them
	if resolvers != nil {
		resolvers.HashResolver.ComputeHashes(model.ExecEventType, &entry.ProcessContext.Process, &entry.ProcessContext.FileEvent)
		if entry.ProcessContext.HasInterpreter() {
			resolvers.HashResolver.ComputeHashes(model.ExecEventType, &entry.ProcessContext.Process, &entry.ProcessContext.LinuxBinprm.FileEvent)
		}
	}
	return &ProcessNode{
		Process:        entry.Process,
		GenerationType: generationType,
		Files:          make(map[string]*FileNode),
		DNSNames:       make(map[string]*DNSNode),
	}
}

// GetChildren returns the list of children from the ProcessNode
func (pn *ProcessNode) GetChildren() *[]*ProcessNode {
	return &pn.Children
}

// GetSiblings returns the list of siblings of the current node
func (pn *ProcessNode) GetSiblings() *[]*ProcessNode {
	if pn.Parent != nil {
		return pn.Parent.GetChildren()
	}
	return nil
}

// GetParent returns nil for the ActivityTree
func (pn *ProcessNode) GetParent() ProcessNodeParent {
	return pn.Parent
}

// AppendChild appends a new root node in the ActivityTree
func (pn *ProcessNode) AppendChild(node *ProcessNode) {
	pn.Children = append(pn.Children, node)
	node.Parent = pn
}

// AppendImageTag appends the given image tag to the list
func (pn *ProcessNode) AppendImageTag(imageTag string) {
	pn.ImageTags, _ = AppendIfNotPresent(pn.ImageTags, imageTag)
}

func (pn *ProcessNode) getNodeLabel(args string) string {
	var label string
	if sprocess.IsBusybox(pn.Process.FileEvent.PathnameStr) {
		arg0, _ := sprocess.GetProcessArgv0(&pn.Process)
		label = fmt.Sprintf("%s %s", arg0, args)
	} else {
		label = fmt.Sprintf("%s %s", pn.Process.FileEvent.PathnameStr, args)
	}
	if len(pn.Process.FileEvent.PkgName) != 0 {
		label += fmt.Sprintf(" \\{%s %s\\}", pn.Process.FileEvent.PkgName, pn.Process.FileEvent.PkgVersion)
	}
	// add hashes
	if len(pn.Process.FileEvent.Hashes) > 0 {
		label += fmt.Sprintf("|%v", strings.Join(pn.Process.FileEvent.Hashes, "|"))
	} else {
		label += fmt.Sprintf("|(%s)", pn.Process.FileEvent.HashState)
	}
	return label
}

// nolint: unused
func (pn *ProcessNode) debug(w io.Writer, prefix string) {
	fmt.Fprintf(w, "%s- process: %s (argv0: %s) (is_exec_exec:%v)\n", prefix, pn.Process.FileEvent.PathnameStr, pn.Process.Argv0, pn.Process.IsExecExec)
	if len(pn.Files) > 0 {
		fmt.Fprintf(w, "%s  files:\n", prefix)
		sortedFiles := make([]*FileNode, 0, len(pn.Files))
		for _, f := range pn.Files {
			sortedFiles = append(sortedFiles, f)
		}
		sort.Slice(sortedFiles, func(i, j int) bool {
			return sortedFiles[i].Name < sortedFiles[j].Name
		})

		for _, f := range sortedFiles {
			f.debug(w, fmt.Sprintf("%s    -", prefix))
		}
	}
	if len(pn.DNSNames) > 0 {
		fmt.Fprintf(w, "%s  dns:\n", prefix)
		for dnsName := range pn.DNSNames {
			fmt.Fprintf(w, "%s    - %s\n", prefix, dnsName)
		}
	}
	if len(pn.Children) > 0 {
		fmt.Fprintf(w, "%s  children:\n", prefix)
		for _, child := range pn.Children {
			child.debug(w, prefix+"    ")
		}
	}
}

// scrubAndReleaseArgsEnvs scrubs the process args and envs, and then releases them
func (pn *ProcessNode) scrubAndReleaseArgsEnvs(resolver *sprocess.EBPFResolver) {
	if pn.Process.ArgsEntry != nil {
		resolver.GetProcessArgvScrubbed(&pn.Process)
		sprocess.GetProcessArgv0(&pn.Process)
		pn.Process.ArgsEntry = nil

	}
	if pn.Process.EnvsEntry != nil {
		resolver.GetProcessEnvs(&pn.Process)
		pn.Process.EnvsEntry = nil
	}
}

// Matches return true if the process fields used to generate the dump are identical with the provided model.Process
func (pn *ProcessNode) Matches(entry *model.Process, matchArgs bool, normalize bool) bool {
	if normalize {
		// should convert /var/run/1234/runc.pid + /var/run/54321/runc.pic into /var/run/*/runc.pid
		match := utils.PathPatternMatch(pn.Process.FileEvent.PathnameStr, entry.FileEvent.PathnameStr, utils.PathPatternMatchOpts{WildcardLimit: 3, PrefixNodeRequired: 1, SuffixNodeRequired: 1, NodeSizeLimit: 8})
		if !match {
			return false
		}
	} else if pn.Process.FileEvent.PathnameStr != entry.FileEvent.PathnameStr {
		return false
	}

	if sprocess.IsBusybox(entry.FileEvent.PathnameStr) {
		panArg0, _ := sprocess.GetProcessArgv0(&pn.Process)
		entryArg0, _ := sprocess.GetProcessArgv0(entry)
		if panArg0 != entryArg0 {
			return false
		}
	}
	if matchArgs {
		panArgs, _ := sprocess.GetProcessArgv(&pn.Process)
		entryArgs, _ := sprocess.GetProcessArgv(entry)
		if len(panArgs) != len(entryArgs) {
			return false
		}
		for i, arg := range panArgs {
			if arg != entryArgs[i] {
				return false
			}
		}
		return true
	}
	return true
}

// InsertSyscalls inserts the syscall of the process in the dump
func (pn *ProcessNode) InsertSyscalls(e *model.Event, syscallMask map[int]int) bool {
	var hasNewSyscalls bool
newSyscallLoop:
	for _, newSyscall := range e.Syscalls.Syscalls {
		for _, existingSyscall := range pn.Syscalls {
			if existingSyscall == int(newSyscall) {
				continue newSyscallLoop
			}
		}

		pn.Syscalls = append(pn.Syscalls, int(newSyscall))
		syscallMask[int(newSyscall)] = int(newSyscall)
		hasNewSyscalls = true
	}
	return hasNewSyscalls
}

// InsertFileEvent inserts the provided file event in the current node. This function returns true if a new entry was
// added, false if the event was dropped.
func (pn *ProcessNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, imageTag string, generationType NodeGenerationType, stats *Stats, dryRun bool, reducer *PathsReducer, resolvers *resolvers.EBPFResolvers) bool {
	var filePath string
	if generationType != Snapshot {
		filePath = event.FieldHandlers.ResolveFilePath(event, fileEvent)
	} else {
		filePath = fileEvent.PathnameStr
	}

	if reducer != nil {
		filePath = reducer.ReducePath(filePath, fileEvent, pn)
	}

	parent, nextParentIndex := ExtractFirstParent(filePath)
	if nextParentIndex == 0 {
		return false
	}

	child, ok := pn.Files[parent]
	if ok {
		return child.InsertFileEvent(fileEvent, event, filePath[nextParentIndex:], imageTag, generationType, stats, dryRun, filePath, resolvers)
	}

	if !dryRun {
		// create new child
		if len(filePath) <= nextParentIndex+1 {
			// this is the last child, add the fileEvent context at the leaf of the files tree.
			node := NewFileNode(fileEvent, event, parent, imageTag, generationType, filePath, resolvers)
			node.MatchedRules = model.AppendMatchedRule(node.MatchedRules, event.Rules)
			stats.FileNodes++
			pn.Files[parent] = node
		} else {
			// This is an intermediary node in the branch that leads to the leaf we want to add. Create a node without the
			// fileEvent context.
			newChild := NewFileNode(nil, nil, parent, imageTag, generationType, filePath, resolvers)
			newChild.InsertFileEvent(fileEvent, event, filePath[nextParentIndex:], imageTag, generationType, stats, dryRun, filePath, resolvers)
			stats.FileNodes++
			pn.Files[parent] = newChild
		}
	}
	return true
}

func (pn *ProcessNode) findDNSNode(DNSName string, DNSMatchMaxDepth int, DNSType uint16) bool {
	if DNSMatchMaxDepth == 0 {
		_, ok := pn.DNSNames[DNSName]
		return ok
	}

	toSearch := dnsFilterSubdomains(DNSName, DNSMatchMaxDepth)
	for name, dnsNode := range pn.DNSNames {
		if dnsFilterSubdomains(name, DNSMatchMaxDepth) == toSearch {
			for _, req := range dnsNode.Requests {
				if req.Type == DNSType {
					return true
				}
			}
		}
	}
	return false
}

// InsertDNSEvent inserts a DNS event in a process node
func (pn *ProcessNode) InsertDNSEvent(evt *model.Event, imageTag string, generationType NodeGenerationType, stats *Stats, DNSNames *utils.StringKeys, dryRun bool, dnsMatchMaxDepth int) bool {
	if dryRun {
		// Use DNSMatchMaxDepth only when searching for a node, not when trying to insert
		return !pn.findDNSNode(evt.DNS.Name, dnsMatchMaxDepth, evt.DNS.Type)
	}

	DNSNames.Insert(evt.DNS.Name)
	dnsNode, ok := pn.DNSNames[evt.DNS.Name]
	if ok {
		// update matched rules
		dnsNode.MatchedRules = model.AppendMatchedRule(dnsNode.MatchedRules, evt.Rules)

		dnsNode.ImageTags, _ = AppendIfNotPresent(dnsNode.ImageTags, imageTag)

		// look for the DNS request type
		for _, req := range dnsNode.Requests {
			if req.Type == evt.DNS.Type {
				return false
			}
		}

		// insert the new request
		dnsNode.Requests = append(dnsNode.Requests, evt.DNS)
		return true
	}

	pn.DNSNames[evt.DNS.Name] = NewDNSNode(&evt.DNS, evt.Rules, generationType, imageTag)
	stats.DNSNodes++
	return true
}

// InsertBindEvent inserts a bind event in a process node
func (pn *ProcessNode) InsertBindEvent(evt *model.Event, imageTag string, generationType NodeGenerationType, stats *Stats, dryRun bool) bool {
	if evt.Bind.SyscallEvent.Retval != 0 {
		return false
	}
	var newNode bool
	evtFamily := model.AddressFamily(evt.Bind.AddrFamily).String()

	// check if a socket of this type already exists
	var sock *SocketNode
	for _, s := range pn.Sockets {
		if s.Family == evtFamily {
			sock = s
		}
	}
	if sock == nil {
		sock = NewSocketNode(evtFamily, generationType)
		if !dryRun {
			stats.SocketNodes++
			pn.Sockets = append(pn.Sockets, sock)
		}
		newNode = true
	}

	// Insert bind event
	if sock.InsertBindEvent(&evt.Bind, imageTag, generationType, evt.Rules, dryRun) {
		newNode = true
	}

	return newNode
}

func (pn *ProcessNode) applyImageTagOnLineageIfNeeded(imageTag string) {
	imageTags, added := AppendIfNotPresent(pn.ImageTags, imageTag)
	if added {
		pn.ImageTags = imageTags
		parent := pn.GetParent()
		for parent != nil {
			parent.AppendImageTag(imageTag)
			parent = parent.GetParent()
		}
	}
}

// TagAllNodes tags this process, its files/dns/socks and childrens with the given image tag
func (pn *ProcessNode) TagAllNodes(imageTag string) {
	if imageTag == "" {
		return
	}

	pn.ImageTags, _ = AppendIfNotPresent(pn.ImageTags, imageTag)
	for _, file := range pn.Files {
		file.tagAllNodes(imageTag)
	}
	for _, dns := range pn.DNSNames {
		dns.appendImageTag(imageTag)
	}
	for _, sock := range pn.Sockets {
		sock.appendImageTag(imageTag)
	}
	for _, child := range pn.Children {
		child.TagAllNodes(imageTag)
	}
}

func removeImageTagFromList(imageTags []string, imageTag string) ([]string, bool) {
	if imageTag == "" {
		return imageTags, false
	}
	removed := false
	return slices.DeleteFunc(imageTags, func(tag string) bool {
		if tag == imageTag {
			removed = true
			return true
		}
		return false
	}), removed
}

// EvictImageTag will remmove every trace of this image tag, and returns true if the process node should be removed
// also, recompute the list of dnsnames and syscalls
func (pn *ProcessNode) EvictImageTag(imageTag string, DNSNames *utils.StringKeys, SyscallsMask map[int]int) bool {
	imageTags, removed := removeImageTagFromList(pn.ImageTags, imageTag)
	if !removed {
		return false // this node don't have the tag, and all his childs/files/dns/etc shouldn't have neither
	}
	if len(imageTags) == 0 {
		// if we removed the last tag, remove entirely the process node from the tree
		return true
	}
	pn.ImageTags = imageTags

	for filename, file := range pn.Files {
		if shouldRemoveNode := file.evictImageTag(imageTag); shouldRemoveNode {
			delete(pn.Files, filename)
		}
	}

	// Evict image tag from dns nodes
	for question, dns := range pn.DNSNames {
		if shouldRemoveNode := dns.evictImageTag(imageTag, DNSNames); shouldRemoveNode {
			delete(pn.DNSNames, question)
		}
	}

	newSockets := []*SocketNode{}
	for _, sock := range pn.Sockets {
		if shouldRemoveNode := sock.evictImageTag(imageTag); !shouldRemoveNode {
			newSockets = append(newSockets, sock)
		}
	}
	pn.Sockets = newSockets

	newChildren := []*ProcessNode{}
	for _, child := range pn.Children {
		if shouldRemoveNode := child.EvictImageTag(imageTag, DNSNames, SyscallsMask); !shouldRemoveNode {
			newChildren = append(newChildren, child)
		}
	}
	pn.Children = newChildren

	for _, id := range pn.Syscalls {
		SyscallsMask[id] = id
	}
	return false
}
