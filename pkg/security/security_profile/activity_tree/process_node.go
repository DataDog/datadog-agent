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
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/pathutils"
)

// ProcessNodeParent is an interface used to identify the parent of a process node
type ProcessNodeParent interface {
	GetParent() ProcessNodeParent
	GetChildren() *[]*ProcessNode
	GetSiblings() *[]*ProcessNode
	AppendChild(node *ProcessNode)
	AppendImageTagID(imageTagID uint64, timestamp time.Time)
}

// ProcessNode holds the activity of a process
type ProcessNode struct {
	NodeBase
	Process        model.Process
	Parent         ProcessNodeParent
	GenerationType NodeGenerationType
	MatchedRules   []*model.MatchedRule

	Files          map[string]*FileNode
	DNSNames       map[string]*DNSNode
	IMDSEvents     map[model.IMDSEvent]*IMDSNode
	NetworkDevices map[model.NetworkDeviceContext]*NetworkDeviceNode

	Sockets      []*SocketNode
	Syscalls     []*SyscallNode
	Capabilities []*CapabilityNode
	Children     []*ProcessNode
}

// size approximates the in-memory heap footprint of this process node, excluding the
// child nodes it owns
func (pn *ProcessNode) size() int64 {
	s := int64(unsafe.Sizeof(*pn))
	s += seenBytes(pn.NodeBase)
	s += processStringsBytes(&pn.Process)

	// Backing arrays for direct-children slices. We charge for the slice slots only;
	// the nodes pointed to are accounted for by their own size() invocations.
	s += sliceBackingBytes(cap(pn.Sockets), unsafe.Sizeof((*SocketNode)(nil)))
	s += sliceBackingBytes(cap(pn.Syscalls), unsafe.Sizeof((*SyscallNode)(nil)))
	s += sliceBackingBytes(cap(pn.Capabilities), unsafe.Sizeof((*CapabilityNode)(nil)))
	s += sliceBackingBytes(cap(pn.Children), unsafe.Sizeof((*ProcessNode)(nil)))
	s += sliceBackingBytes(cap(pn.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))

	// Map bucket overhead. We use stringMapBytes for string-keyed maps (it adds the key
	// content too) and fixedKeyMapBytes for struct-keyed maps where the key has no heap.
	s += stringMapBytes(pn.Files)
	s += stringMapBytes(pn.DNSNames)
	s += fixedKeyMapBytes(pn.IMDSEvents)
	s += fixedKeyMapBytes(pn.NetworkDevices)
	return s
}

// NewProcessNode returns a new ProcessNode instance
func NewProcessNode(entry *model.ProcessCacheEntry, generationType NodeGenerationType, resolvers *resolvers.EBPFResolvers) *ProcessNode {
	// call the callback to resolve additional fields before copying them
	if resolvers != nil {
		resolvers.HashResolver.ComputeHashes(model.ExecEventType, &entry.ProcessContext.Process, &entry.ProcessContext.FileEvent, 0)
		if entry.ProcessContext.HasInterpreter() {
			resolvers.HashResolver.ComputeHashes(model.ExecEventType, &entry.ProcessContext.Process, &entry.ProcessContext.LinuxBinprm.FileEvent, 0)
		}
	}
	node := &ProcessNode{
		Process:        entry.Process,
		GenerationType: generationType,
		Files:          make(map[string]*FileNode),
		DNSNames:       make(map[string]*DNSNode),
		IMDSEvents:     make(map[model.IMDSEvent]*IMDSNode),
		NetworkDevices: make(map[model.NetworkDeviceContext]*NetworkDeviceNode),
	}
	node.NodeBase = NewNodeBase()

	return node
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

func (pn *ProcessNode) getNodeLabel(args string) string {
	var builder strings.Builder
	builder.WriteString(tableHeader)

	builder.WriteString("<TR><TD>Command</TD><TD><FONT POINT-SIZE=\"" + strconv.Itoa(bigText) + "\">")
	var cmd string
	if sprocess.IsBusybox(pn.Process.FileEvent.PathnameStr) {
		arg0, _ := sprocess.GetProcessArgv0(&pn.Process)
		cmd = fmt.Sprintf("%s %s", arg0, args)
	} else {
		cmd = fmt.Sprintf("%s %s", pn.Process.FileEvent.PathnameStr, args)
	}
	if len(cmd) > 100 {
		cmd = cmd[:100] + " ..."
	}
	builder.WriteString(html.EscapeString(cmd))
	builder.WriteString("</FONT></TD></TR>")

	if len(pn.Process.FileEvent.PkgName) != 0 {
		builder.WriteString("<TR><TD>Package</TD><TD>" + pn.Process.FileEvent.PkgName + ":" + pn.Process.FileEvent.PkgVersion + "</TD></TR>")
	}
	// add hashes
	if len(pn.Process.FileEvent.Hashes) > 0 {
		builder.WriteString("<TR><TD>Hashes</TD><TD>" + pn.Process.FileEvent.Hashes[0] + "</TD></TR>")
		for _, h := range pn.Process.FileEvent.Hashes {
			builder.WriteString("<TR><TD></TD><TD>" + h + "</TD></TR>")
		}
	} else {
		builder.WriteString("<TR><TD>Hash state</TD><TD>" + pn.Process.FileEvent.HashState.String() + "</TD></TR>")
	}
	builder.WriteString("</TABLE>>")
	return builder.String()
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
			f.debug(w, prefix+"    -")
		}
	}
	if len(pn.DNSNames) > 0 {
		fmt.Fprintf(w, "%s  dns:\n", prefix)
		for dnsName := range pn.DNSNames {
			fmt.Fprintf(w, "%s    - %s\n", prefix, dnsName)
		}
	}
	if len(pn.IMDSEvents) > 0 {
		fmt.Fprintf(w, "%s  imds:\n", prefix)
		for evt := range pn.IMDSEvents {
			fmt.Fprintf(w, "%s    - %s | %s\n", prefix, evt.CloudProvider, evt.Type)
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
		match := pathutils.PathPatternMatch(pn.Process.FileEvent.PathnameStr, entry.FileEvent.PathnameStr, pathutils.PathPatternMatchOpts{WildcardLimit: 3, PrefixNodeRequired: 1, SuffixNodeRequired: 1, NodeSizeLimit: 8})
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
func (pn *ProcessNode) InsertSyscalls(e *model.Event, imageTagID uint64, syscallMask map[int]int, stats *Stats, dryRun bool) bool {
	var hasNewSyscalls bool
newSyscallLoop:
	for _, newSyscall := range e.Syscalls.Syscalls {
		for _, existingSyscall := range pn.Syscalls {
			if existingSyscall.Syscall == int(newSyscall) {
				existingSyscall.AppendImageTagID(imageTagID, e.ResolveEventTime())
				continue newSyscallLoop
			}
		}

		hasNewSyscalls = true
		if dryRun {
			// exit early
			break
		}
		sn := NewSyscallNode(int(newSyscall), e.ResolveEventTime(), imageTagID, Runtime)
		pn.Syscalls = append(pn.Syscalls, sn)
		syscallMask[int(newSyscall)] = int(newSyscall)
		stats.SyscallNodes++
		stats.SizeBytes += sn.size()
	}

	return hasNewSyscalls
}

// InsertFileEvent inserts the provided file event in the current node. Returns whether a new entry was
// added and the NodeBase of the leaf FileNode reached or created.
func (pn *ProcessNode) InsertFileEvent(fileEvent *model.FileEvent, event *model.Event, imageTagID uint64, generationType NodeGenerationType, stats *Stats, dryRun bool, reducer *PathsReducer, resolvers *resolvers.EBPFResolvers) (bool, *NodeBase) {
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
		return false, nil
	}

	child, ok := pn.Files[parent]
	if ok {
		return child.InsertFileEvent(fileEvent, event, filePath[nextParentIndex:], imageTagID, generationType, stats, dryRun, filePath, resolvers)
	}

	if !dryRun {
		if len(filePath) <= nextParentIndex+1 {
			// this is the last child, add the fileEvent context at the leaf of the files tree.
			node := NewFileNode(fileEvent, event, parent, imageTagID, generationType, filePath, resolvers)
			node.MatchedRules = model.AppendMatchedRule(node.MatchedRules, event.Rules)
			stats.FileNodes++
			stats.SizeBytes += node.size()
			pn.Files[parent] = node
			return true, &node.NodeBase
		}
		newChild := NewFileNode(nil, nil, parent, imageTagID, generationType, filePath, resolvers)
		_, leafNodeBase := newChild.InsertFileEvent(fileEvent, event, filePath[nextParentIndex:], imageTagID, generationType, stats, dryRun, filePath, resolvers)
		stats.FileNodes++
		stats.SizeBytes += newChild.size()
		pn.Files[parent] = newChild
		return true, leafNodeBase
	}
	return true, nil
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
				if req.Question.Type == DNSType {
					return true
				}
			}
		}
	}
	return false
}

// InsertDNSEvent inserts a DNS event in a process node
func (pn *ProcessNode) InsertDNSEvent(evt *model.Event, imageTagID uint64, generationType NodeGenerationType, stats *Stats, DNSNames *utils.StringKeys, dryRun bool, dnsMatchMaxDepth int) bool {
	if dryRun {
		// Use DNSMatchMaxDepth only when searching for a node, not when trying to insert
		return !pn.findDNSNode(evt.DNS.Question.Name, dnsMatchMaxDepth, evt.DNS.Question.Type)
	}

	DNSNames.Insert(evt.DNS.Question.Name)
	dnsNode, ok := pn.DNSNames[evt.DNS.Question.Name]
	if ok {
		// update matched rules
		dnsNode.MatchedRules = model.AppendMatchedRule(dnsNode.MatchedRules, evt.Rules)

		dnsNode.AppendImageTagID(imageTagID, evt.ResolveEventTime())

		// look for the DNS request type
		for _, req := range dnsNode.Requests {
			if req.Question.Type == evt.DNS.Question.Type {
				return false
			}
		}

		sizeBefore := dnsNode.size()
		dnsNode.Requests = append(dnsNode.Requests, evt.DNS)
		stats.SizeBytes += dnsNode.size() - sizeBefore
		return true
	}

	dnsNode = NewDNSNode(&evt.DNS, evt, evt.Rules, generationType, imageTagID)
	pn.DNSNames[evt.DNS.Question.Name] = dnsNode
	stats.DNSNodes++
	stats.SizeBytes += dnsNode.size()
	return true
}

// InsertIMDSEvent inserts an IMDS event in a process node
func (pn *ProcessNode) InsertIMDSEvent(evt *model.Event, imageTagID uint64, generationType NodeGenerationType, stats *Stats, dryRun bool) bool {
	imdsNode, ok := pn.IMDSEvents[evt.IMDS]
	if ok {
		imdsNode.MatchedRules = model.AppendMatchedRule(imdsNode.MatchedRules, evt.Rules)
		imdsNode.AppendImageTagID(imageTagID, evt.ResolveEventTime())
		return false
	}

	if !dryRun {
		// create new node
		imdsNode := NewIMDSNode(&evt.IMDS, evt, evt.Rules, generationType, imageTagID)
		pn.IMDSEvents[evt.IMDS] = imdsNode
		stats.IMDSNodes++
		stats.SizeBytes += imdsNode.size()
	}
	return true
}

// InsertNetworkFlowMonitorEvent inserts a Network Flow Monitor event in a process node
func (pn *ProcessNode) InsertNetworkFlowMonitorEvent(evt *model.Event, imageTagID uint64, generationType NodeGenerationType, stats *Stats, dryRun bool) bool {
	deviceNode, ok := pn.NetworkDevices[evt.NetworkFlowMonitor.Device]
	if ok {
		return deviceNode.insertNetworkFlowMonitorEvent(&evt.NetworkFlowMonitor, evt, dryRun, evt.Rules, generationType, imageTagID, stats)
	}

	if !dryRun {
		newNode := NewNetworkDeviceNode(&evt.NetworkFlowMonitor.Device, generationType)
		pn.NetworkDevices[evt.NetworkFlowMonitor.Device] = newNode
		// Charge for the device struct itself before its first flow is inserted; the
		// flow's own size is added by insertNetworkFlowMonitorEvent below.
		stats.SizeBytes += newNode.size()
		newNode.insertNetworkFlowMonitorEvent(&evt.NetworkFlowMonitor, evt, dryRun, evt.Rules, generationType, imageTagID, stats)
	}
	return true
}

// InsertBindEvent inserts a bind event in a process node. Returns whether a new entry was
// added and the NodeBase of the matched or newly created BindNode.
func (pn *ProcessNode) InsertBindEvent(evt *model.Event, imageTagID uint64, generationType NodeGenerationType, stats *Stats, dryRun bool) (bool, *NodeBase) {
	if evt.Bind.SyscallEvent.Retval != 0 {
		return false, nil
	}
	var newNode bool
	evtFamily := model.AddressFamily(evt.Bind.AddrFamily).String()

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
			stats.SizeBytes += sock.size()
			pn.Sockets = append(pn.Sockets, sock)
		}
		newNode = true
	}

	bindNew, bindNodeBase := sock.InsertBindEvent(&evt.Bind, evt, imageTagID, generationType, evt.Rules, stats, dryRun)
	if bindNew {
		newNode = true
	}

	return newNode, bindNodeBase
}

// InsertCapabilitiesUsageEvent inserts a capabilities usage event in a process node
func (pn *ProcessNode) InsertCapabilitiesUsageEvent(evt *model.Event, imageTagID uint64, stats *Stats, dryRun bool) bool {
	hasNewCapabilitiesUsage := false
nextCapability:
	for capability := uint64(0); capability <= unix.CAP_LAST_CAP; capability++ {
		if evt.CapabilitiesUsage.Attempted&(1<<capability) == 0 {
			continue
		}

		capable := evt.CapabilitiesUsage.Used&(1<<capability) != 0

		for _, existingCapabilityNode := range pn.Capabilities {
			if existingCapabilityNode.Capability == capability && existingCapabilityNode.Capable == capable {
				existingCapabilityNode.AppendImageTagID(imageTagID, evt.ResolveEventTime())
				continue nextCapability
			}
		}

		hasNewCapabilitiesUsage = true
		if dryRun {
			break
		}

		capabilityNode := NewCapabilityNode(capability, capable, evt.ResolveEventTime(), imageTagID, Runtime)
		pn.Capabilities = append(pn.Capabilities, capabilityNode)
		stats.CapabilityNodes++
		stats.SizeBytes += capabilityNode.size()
	}

	return hasNewCapabilitiesUsage
}

func (pn *ProcessNode) applyImageTagOnLineageIfNeeded(imageTagID uint64) {
	if pn.HasImageTag(imageTagID) {
		return
	}
	pn.AppendImageTagID(imageTagID, pn.Process.ExecTime)
	parent := pn.GetParent()
	for parent != nil {
		parent.AppendImageTagID(imageTagID, pn.Process.ExecTime)
		parent = parent.GetParent()
	}
}

// TagAllNodes tags this process, its files/dns/socks and childrens with the given image tag
func (pn *ProcessNode) TagAllNodes(imageTagID uint64, timestamp time.Time) {
	if imageTagID == 0 {
		return
	}

	pn.AppendImageTagID(imageTagID, timestamp)
	for _, file := range pn.Files {
		file.tagAllNodes(imageTagID, timestamp)
	}
	for _, dns := range pn.DNSNames {
		dns.AppendImageTagID(imageTagID, timestamp)
	}
	for _, sock := range pn.Sockets {
		sock.AppendImageTagID(imageTagID, timestamp)
	}
	for _, scall := range pn.Syscalls {
		scall.AppendImageTagID(imageTagID, timestamp)
	}
	for _, imds := range pn.IMDSEvents {
		imds.AppendImageTagID(imageTagID, timestamp)
	}
	for _, device := range pn.NetworkDevices {
		device.appendImageTag(imageTagID, timestamp)
	}
	for _, capabilityNode := range pn.Capabilities {
		capabilityNode.AppendImageTagID(imageTagID, timestamp)
	}
	for _, child := range pn.Children {
		child.TagAllNodes(imageTagID, timestamp)
	}
}

// EvictImageTag will remove every trace of this image tag, and returns true if the process node should be removed
// also, recompute the list of dnsnames and syscalls
func (pn *ProcessNode) EvictImageTag(imageTagID uint64, DNSNames *utils.StringKeys, SyscallsMask map[int]int) (bool, int64) {
	if !pn.HasImageTag(imageTagID) {
		return false, 0 // this node doesn't have the tag, and all its children/files/dns/etc shouldn't have it either
	}
	IsNodeEmpty := pn.NodeBase.EvictImageTag(imageTagID)
	if IsNodeEmpty {
		// if we removed the last tag, remove entirely the process node from the tree
		return true, processSubtreeSizeBytes(pn)
	}

	var removed int64

	for filename, file := range pn.Files {
		shouldRemove, fileRemoved := file.evictImageTag(imageTagID)
		if shouldRemove {
			delete(pn.Files, filename)
		}
		removed += fileRemoved
	}

	// Evict image tag from dns nodes
	for question, dns := range pn.DNSNames {
		if shouldRemoveNode := dns.evictImageTag(imageTagID, DNSNames); shouldRemoveNode {
			removed += dns.size()
			delete(pn.DNSNames, question)
		}
	}

	// Evict image tag from IMDS nodes
	for key, imds := range pn.IMDSEvents {
		if shouldRemoveNode := imds.EvictImageTag(imageTagID); shouldRemoveNode {
			removed += imds.size()
			delete(pn.IMDSEvents, key)
		}
	}

	// Evict image tag from network device nodes
	for key, device := range pn.NetworkDevices {
		shouldRemove, deviceRemoved := device.evictImageTag(imageTagID)
		removed += deviceRemoved
		if shouldRemove {
			removed += device.size()
			delete(pn.NetworkDevices, key)
		}
	}

	newSockets := []*SocketNode{}
	for _, sock := range pn.Sockets {
		shouldRemoveNode, bindBytes := sock.evictImageTag(imageTagID)
		removed += bindBytes
		if shouldRemoveNode {
			removed += sock.size()
			continue
		}
		newSockets = append(newSockets, sock)
	}
	pn.Sockets = newSockets

	newSyscalls := []*SyscallNode{}
	for _, scall := range pn.Syscalls {
		if shouldRemove := scall.EvictImageTag(imageTagID); !shouldRemove {
			newSyscalls = append(newSyscalls, scall)
			SyscallsMask[scall.Syscall] = scall.Syscall
		} else {
			removed += scall.size()
		}
	}
	pn.Syscalls = newSyscalls

	var newCapabilities []*CapabilityNode
	for _, capabilityNode := range pn.Capabilities {
		if shouldRemove := capabilityNode.EvictImageTag(imageTagID); !shouldRemove {
			newCapabilities = append(newCapabilities, capabilityNode)
		} else {
			removed += capabilityNode.size()
		}
	}
	pn.Capabilities = newCapabilities

	newChildren := []*ProcessNode{}
	for _, child := range pn.Children {
		shouldRemoveNode, childRemoved := child.EvictImageTag(imageTagID, DNSNames, SyscallsMask)
		if !shouldRemoveNode {
			newChildren = append(newChildren, child)
		}
		removed += childRemoved
	}
	pn.Children = newChildren
	return false, removed
}

// EvictUnusedNodes evicts all child nodes that haven't been touched since the given timestamp
// and returns the total number of process nodes evicted and the total bytes freed.
// A node is only evicted if all its children are evictable.
// profileImageTagID is the pre-resolved internal ID for the profile's image tag (0 means unknown/no tag).
func (pn *ProcessNode) EvictUnusedNodes(before time.Time, filepathsInProcessCache map[ImageProcessKey]bool, profileImageName string, profileImageTag string, profileImageTagID uint64) (int, int64) {
	totalEvicted := 0
	var removedBytes int64

	key := ImageProcessKey{
		ImageName: profileImageName,
		ImageTag:  profileImageTag,
	}

	// First, recursively evict unused nodes from children
	for i := len(pn.Children) - 1; i >= 0; i-- {
		child := pn.Children[i]
		evicted, childRemoved := child.EvictUnusedNodes(before, filepathsInProcessCache, profileImageName, profileImageTag, profileImageTagID)
		totalEvicted += evicted
		removedBytes += childRemoved

		// If the child process node itself has no image tags left after eviction, remove it entirely.
		if child.SeenIsEmpty() {
			removedBytes += processSubtreeSizeBytes(child)
			pn.Children = append(pn.Children[:i], pn.Children[i+1:]...)
			totalEvicted++
		}
	}

	// Try a fallback if the node is in the process cache
	// Check if this specific image/tag/filepath combination exists in the cache
	// The filepath might not be sufficient to uniquely identify the node, but it's a good enough approximation for now
	// Edge case: foo->bar->foo, if the second foo is no longer in the process cache, it will still be refreshed because of the first foo
	key.Filepath = pn.Process.FileEvent.PathnameStr

	if filepathsInProcessCache[key] && profileImageTagID != 0 {
		// check if the node was supposed to be removed, then update the last seen to now
		if elem, ok := pn.GetSeenTimes(profileImageTagID); ok && elem.LastSeen.Before(before) {
			pn.NodeBase.AppendImageTagID(profileImageTagID, time.Now())
		}
	}

	_ = pn.NodeBase.EvictBeforeTimestamp(before)

	// If the process node itself can be evicted, return early.
	// The caller will subtract the remaining subtree size when it removes this node.
	if len(pn.Children) == 0 && pn.SeenIsEmpty() {
		return totalEvicted, removedBytes
	}

	// Evict unused syscall nodes
	for i := len(pn.Syscalls) - 1; i >= 0; i-- {
		syscallNode := pn.Syscalls[i]
		if syscallNode.NodeBase.EvictBeforeTimestamp(before) > 0 {
			if syscallNode.SeenIsEmpty() {
				removedBytes += syscallNode.size()
				pn.Syscalls = append(pn.Syscalls[:i], pn.Syscalls[i+1:]...)
			}
		}
	}

	// Evict unused file nodes
	for path, fileNode := range pn.Files {
		if fileNode.NodeBase.EvictBeforeTimestamp(before) > 0 {
			if fileNode.SeenIsEmpty() {
				removedBytes += fileSubtreeSizeBytes(fileNode)
				delete(pn.Files, path)
			}
		}
	}

	// Evict unused DNS nodes
	for name, dnsNode := range pn.DNSNames {
		if dnsNode.NodeBase.EvictBeforeTimestamp(before) > 0 {
			if dnsNode.SeenIsEmpty() {
				removedBytes += dnsNode.size()
				delete(pn.DNSNames, name)
			}
		}
	}

	// Evict unused IMDS nodes
	for event, imdsNode := range pn.IMDSEvents {
		if imdsNode.NodeBase.EvictBeforeTimestamp(before) > 0 {
			if imdsNode.SeenIsEmpty() {
				removedBytes += imdsNode.size()
				delete(pn.IMDSEvents, event)
			}
		}
	}

	// Note: NetworkDeviceNode doesn't embed NodeBase so we skip eviction for network devices

	// Evict unused socket nodes
	for i := len(pn.Sockets) - 1; i >= 0; i-- {
		socketNode := pn.Sockets[i]
		if socketNode.NodeBase.EvictBeforeTimestamp(before) > 0 {
			if socketNode.SeenIsEmpty() {
				removedBytes += socketNode.size()
				pn.Sockets = append(pn.Sockets[:i], pn.Sockets[i+1:]...)
			}
		}
	}

	// Evict unused capability nodes
	for i := len(pn.Capabilities) - 1; i >= 0; i-- {
		capabilityNode := pn.Capabilities[i]
		if capabilityNode.NodeBase.EvictBeforeTimestamp(before) > 0 {
			if capabilityNode.SeenIsEmpty() {
				removedBytes += capabilityNode.size()
				pn.Capabilities = append(pn.Capabilities[:i], pn.Capabilities[i+1:]...)
			}
		}
	}

	return totalEvicted, removedBytes
}
