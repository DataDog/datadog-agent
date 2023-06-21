// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activity_tree

import (
	"fmt"
	"io"
	"sort"

	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ProcessNode holds the activity of a process
type ProcessNode struct {
	Process        model.Process
	GenerationType NodeGenerationType
	MatchedRules   []*model.MatchedRule

	Files    map[string]*FileNode
	DNSNames map[string]*DNSNode
	Sockets  []*SocketNode
	Syscalls []int
	Children []*ProcessNode
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
	return label
}

// NewProcessNode returns a new ProcessNode instance
func NewProcessNode(entry *model.ProcessCacheEntry, generationType NodeGenerationType) *ProcessNode {
	return &ProcessNode{
		Process:        entry.Process,
		GenerationType: generationType,
		Files:          make(map[string]*FileNode),
		DNSNames:       make(map[string]*DNSNode),
	}
}

// nolint: unused
func (pn *ProcessNode) debug(w io.Writer, prefix string) {
	fmt.Fprintf(w, "%s- process: %s\n", prefix, pn.Process.FileEvent.PathnameStr)
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
func (pn *ProcessNode) scrubAndReleaseArgsEnvs(resolver *sprocess.Resolver) {
	if pn.Process.ArgsEntry != nil {
		resolver.GetProcessScrubbedArgv(&pn.Process)
		sprocess.GetProcessArgv0(&pn.Process)
		pn.Process.ArgsEntry = nil

	}
	if pn.Process.EnvsEntry != nil {
		resolver.GetProcessEnvs(&pn.Process)
		pn.Process.EnvsEntry = nil
	}
}

// Matches return true if the process fields used to generate the dump are identical with the provided model.Process
func (pn *ProcessNode) Matches(entry *model.Process, matchArgs bool) bool {
	if pn.Process.FileEvent.PathnameStr == entry.FileEvent.PathnameStr {
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
	return false
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
func (pn *ProcessNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, generationType NodeGenerationType, stats *ActivityTreeStats, dryRun bool, reducer *PathsReducer) bool {
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
		return child.InsertFileEvent(fileEvent, event, filePath[nextParentIndex:], generationType, stats, dryRun, filePath)
	}

	if !dryRun {
		// create new child
		if len(fileEvent.PathnameStr) <= nextParentIndex+1 {
			// this is the last child, add the fileEvent context at the leaf of the files tree.
			node := NewFileNode(fileEvent, event, parent, generationType, filePath)
			node.MatchedRules = model.AppendMatchedRule(node.MatchedRules, event.Rules)
			stats.FileNodes++
			pn.Files[parent] = node
		} else {
			// This is an intermediary node in the branch that leads to the leaf we want to add. Create a node without the
			// fileEvent context.
			newChild := NewFileNode(nil, nil, parent, generationType, filePath)
			newChild.InsertFileEvent(fileEvent, event, fileEvent.PathnameStr[nextParentIndex:], generationType, stats, dryRun, filePath)
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
func (pn *ProcessNode) InsertDNSEvent(evt *model.Event, generationType NodeGenerationType, stats *ActivityTreeStats, DNSNames *utils.StringKeys, dryRun bool, dnsMatchMaxDepth int) bool {
	if dryRun {
		// Use DNSMatchMaxDepth only when searching for a node, not when trying to insert
		return !pn.findDNSNode(evt.DNS.Name, dnsMatchMaxDepth, evt.DNS.Type)
	}

	DNSNames.Insert(evt.DNS.Name)
	dnsNode, ok := pn.DNSNames[evt.DNS.Name]
	if ok {
		// update matched rules
		dnsNode.MatchedRules = model.AppendMatchedRule(dnsNode.MatchedRules, evt.Rules)

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

	pn.DNSNames[evt.DNS.Name] = NewDNSNode(&evt.DNS, evt.Rules, generationType)
	stats.DNSNodes++
	return true
}

// InsertBindEvent inserts a bind event in a process node
func (pn *ProcessNode) InsertBindEvent(evt *model.Event, generationType NodeGenerationType, stats *ActivityTreeStats, dryRun bool) bool {
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
	if sock.InsertBindEvent(&evt.Bind, generationType, evt.Rules, dryRun) {
		newNode = true
	}

	return newNode
}
