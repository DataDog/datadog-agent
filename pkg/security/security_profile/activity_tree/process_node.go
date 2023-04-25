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
	label := fmt.Sprintf("%s %s", pn.Process.FileEvent.PathnameStr, args)
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
		_, _ = resolver.GetProcessScrubbedArgv(&pn.Process)
		pn.Process.Argv0, _ = resolver.GetProcessArgv0(&pn.Process)
		pn.Process.ArgsEntry = nil

	}
	if pn.Process.EnvsEntry != nil {
		envs, envsTruncated := resolver.GetProcessEnvs(&pn.Process)
		pn.Process.Envs = envs
		pn.Process.EnvsTruncated = envsTruncated
		pn.Process.EnvsEntry = nil
	}
}

// Matches return true if the process fields used to generate the dump are identical with the provided model.Process
func (pn *ProcessNode) Matches(entry *model.Process, matchArgs bool) bool {
	if pn.Process.FileEvent.PathnameStr == entry.FileEvent.PathnameStr {
		if matchArgs {
			var panArgs, entryArgs []string
			if pn.Process.ArgsEntry != nil {
				panArgs, _ = sprocess.GetProcessArgv(&pn.Process)
			} else {
				panArgs = pn.Process.Argv
			}
			if entry.ArgsEntry != nil {
				entryArgs, _ = sprocess.GetProcessArgv(entry)
			} else {
				entryArgs = entry.Argv
			}
			if len(panArgs) != len(entryArgs) {
				return false
			}

			var found bool
			for _, arg1 := range panArgs {
				found = false
				for _, arg2 := range entryArgs {
					if arg1 == arg2 {
						found = true
						break
					}
				}
				if !found {
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
func (pn *ProcessNode) InsertSyscalls(e *model.Event, syscallMask map[int]int) (bool, error) {
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
	return hasNewSyscalls, nil
}

// InsertFileEvent inserts the provided file event in the current node. This function returns true if a new entry was
// added, false if the event was dropped.
func (pn *ProcessNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, generationType NodeGenerationType, stats *ActivityTreeStats, shouldMergePaths bool, shadowInsertion bool) (bool, error) {
	var filePath string
	if generationType != Snapshot {
		filePath = event.FieldHandlers.ResolveFilePath(event, fileEvent)
	} else {
		filePath = fileEvent.PathnameStr
	}

	parent, nextParentIndex := ExtractFirstParent(filePath)
	if nextParentIndex == 0 {
		return false, nil
	}

	child, ok := pn.Files[parent]
	if ok {
		return child.InsertFileEvent(fileEvent, event, fileEvent.PathnameStr[nextParentIndex:], generationType, stats, shouldMergePaths, shadowInsertion), nil
	}

	if !shadowInsertion {
		// create new child
		if len(fileEvent.PathnameStr) <= nextParentIndex+1 {
			// this is the last child, add the fileEvent context at the leaf of the files tree.
			node := NewFileNode(fileEvent, event, parent, generationType)
			node.MatchedRules = model.AppendMatchedRule(node.MatchedRules, event.Rules)
			stats.fileNodes++
			pn.Files[parent] = node
		} else {
			// This is an intermediary node in the branch that leads to the leaf we want to add. Create a node without the
			// fileEvent context.
			newChild := NewFileNode(nil, nil, parent, generationType)
			newChild.InsertFileEvent(fileEvent, event, fileEvent.PathnameStr[nextParentIndex:], generationType, stats, shouldMergePaths, shadowInsertion)
			stats.fileNodes++
			pn.Files[parent] = newChild
		}
	}
	return true, nil
}

// InsertDNSEvent inserts a DNS event in a process node
func (pn *ProcessNode) InsertDNSEvent(evt *model.Event, generationType NodeGenerationType, stats *ActivityTreeStats, DNSNames *utils.StringKeys, shadowInsertion bool) (bool, error) {
	DNSNames.Insert(evt.DNS.Name)

	if dnsNode, ok := pn.DNSNames[evt.DNS.Name]; ok {
		// update matched rules
		if !shadowInsertion {
			dnsNode.MatchedRules = model.AppendMatchedRule(dnsNode.MatchedRules, evt.Rules)
		}

		// look for the DNS request type
		for _, req := range dnsNode.Requests {
			if req.Type == evt.DNS.Type {
				return false, nil
			}
		}

		if !shadowInsertion {
			// insert the new request
			dnsNode.Requests = append(dnsNode.Requests, evt.DNS)
		}
		return true, nil
	}

	if !shadowInsertion {
		pn.DNSNames[evt.DNS.Name] = NewDNSNode(&evt.DNS, evt.Rules, generationType)
		stats.dnsNodes++
	}
	return true, nil
}

// InsertBindEvent inserts a bind event in a process node
func (pn *ProcessNode) InsertBindEvent(evt *model.Event, generationType NodeGenerationType, stats *ActivityTreeStats, shadowInsertion bool) (bool, error) {
	if evt.Bind.SyscallEvent.Retval != 0 {
		return false, nil
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
		if !shadowInsertion {
			stats.socketNodes++
			pn.Sockets = append(pn.Sockets, sock)
		}
		newNode = true
	}

	// Insert bind event
	if sock.InsertBindEvent(&evt.Bind, generationType, evt.Rules, shadowInsertion) {
		newNode = true
	}

	return newNode, nil
}
