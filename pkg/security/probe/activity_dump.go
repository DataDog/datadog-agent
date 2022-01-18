// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/DataDog/gopsutil/process"
	"github.com/cilium/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ActivityDump holds the activity tree for the workload defined by the provided list of tags
type ActivityDump struct {
	Tags                []string                        `json:"tags,omitempty"`
	Comm                string                          `json:"comm,omitempty"`
	Start               time.Time                       `json:"start"`
	Timeout             time.Duration                   `json:"duration"`
	End                 time.Time                       `json:"end"`
	CookiesNode         map[uint32]*ProcessActivityNode `json:"-"`
	ProcessActivityTree []*ProcessActivityNode          `json:"tree"`

	OutputFile string `json:"-"`
	outputFile *os.File
	GraphFile  string `json:"-"`
	graphFile  *os.File
	tracedPIDs *ebpf.Map
	resolvers  *Resolvers
	scrubber   *config.DataScrubber

	differentiateArgs bool
	processedCount    map[model.EventType]*uint64
	addedCount        map[model.EventType]*uint64
}

// NewActivityDump returns a new instance of an ActivityDump
func NewActivityDump(params *api.DumpActivityParams, tracedPIDs *ebpf.Map, resolvers *Resolvers, scrubber *config.DataScrubber) (*ActivityDump, error) {
	var err error

	ad := ActivityDump{
		Tags:              params.Tags,
		Comm:              params.Comm,
		CookiesNode:       make(map[uint32]*ProcessActivityNode),
		Start:             time.Now(),
		Timeout:           time.Duration(params.Timeout) * time.Minute,
		tracedPIDs:        tracedPIDs,
		resolvers:         resolvers,
		scrubber:          scrubber,
		differentiateArgs: params.DifferentiateArgs,
		processedCount:    make(map[model.EventType]*uint64),
		addedCount:        make(map[model.EventType]*uint64),
	}

	// generate counters
	for i := model.EventType(0); i < model.MaxEventType; i++ {
		processed := uint64(0)
		ad.processedCount[i] = &processed
		added := uint64(0)
		ad.addedCount[i] = &added
	}

	// generate random output file
	ad.outputFile, err = ioutil.TempFile("/tmp", "activity-dump-")
	if err != nil {
		return nil, err
	}

	if err = os.Chmod(ad.outputFile.Name(), 0400); err != nil {
		return nil, err
	}
	ad.OutputFile = ad.outputFile.Name()

	// generate random graph file
	if params.WithGraph {
		ad.graphFile, err = ioutil.TempFile("/tmp", "graph-dump-")
		if err != nil {
			return nil, err
		}

		if err = os.Chmod(ad.graphFile.Name(), 0400); err != nil {
			return nil, err
		}
		ad.GraphFile = ad.graphFile.Name()
	}
	return &ad, nil
}

// TagsListMatches returns true if the ActivityDump tags list matches the provided list of tags
func (ad *ActivityDump) TagsListMatches(tags []string) bool {
	var found bool
	for _, adTag := range ad.Tags {
		found = false
		for _, tag := range tags {
			if adTag == tag {
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

// CommMatches returns true if the ActivityDump comm matches the provided comm
func (ad *ActivityDump) CommMatches(comm string) bool {
	return ad.Comm == comm
}

// Matches returns true if the provided list of tags and / or the provided comm match the current ActivityDump
func (ad *ActivityDump) Matches(entry *model.ProcessCacheEntry) bool {
	if entry == nil {
		return false
	}

	if len(ad.Comm) > 0 {
		if !ad.CommMatches(entry.Comm) {
			return false
		}
	}

	if len(ad.Tags) > 0 {
		if !ad.TagsListMatches(ad.resolvers.ResolvePCEContainerTags(entry)) {
			return false
		}
	}

	return true
}

// Done stops an active dump
func (ad *ActivityDump) Done() {
	ad.End = time.Now()
	//ad.debug()
	ad.dump()
	_ = ad.outputFile.Close()
	if ad.graphFile != nil {
		title := "Activity tree"
		if len(ad.Tags) > 0 {
			title = fmt.Sprintf("%s [%s]", title, strings.Join(ad.Tags, " "))
		}
		if len(ad.Comm) > 0 {
			title = fmt.Sprintf("%s Comm(%s)", title, ad.Comm)
		}
		err := ad.generateGraph(title)
		if err != nil {
			seclog.Errorf("couldn't generate activity graph: %s", err)
		}
	}
	_ = ad.graphFile.Close()

	// release all shared resources
	for _, p := range ad.ProcessActivityTree {
		p.recursiveRelease()
	}
}

func (ad *ActivityDump) dump() {
	raw, err := json.Marshal(ad)
	if err != nil {
		seclog.Errorf("couldn't marshal ActivityDump: %s\n", err)
		return
	}
	_, err = ad.outputFile.Write(raw)
	if err != nil {
		seclog.Errorf("couldn't write ActivityDump: %s\n", err)
		return
	}
}

// nolint: unused
func (ad *ActivityDump) debug() {
	for _, root := range ad.ProcessActivityTree {
		root.debug("")
	}
}

// Insert inserts the provided event in the active ActivityDump. This function returns true if a new entry was added,
// false if the event was dropped.
func (ad *ActivityDump) Insert(event *Event) (newEntry bool) {
	// ignore fork events for now
	if event.GetEventType() == model.ForkEventType {
		return false
	}

	// metrics
	atomic.AddUint64(ad.processedCount[event.GetEventType()], 1)
	defer func() {
		if newEntry {
			atomic.AddUint64(ad.addedCount[event.GetEventType()], 1)
		}
	}()

	// find the node where the event should be inserted
	node := ad.FindOrCreateProcessActivityNode(event.ResolveProcessCacheEntry())
	if node == nil {
		// a process node couldn't be found for the provided event as it doesn't match the ActivityDump query
		return false
	}

	// insert the event based on its type
	switch event.GetEventType() {
	case model.FileOpenEventType:
		return node.InsertOpen(event)
	}
	return false
}

// FindOrCreateProcessActivityNode finds or a create a new process activity node in the activity dump if the entry
// matches the activity dump selector.
func (ad *ActivityDump) FindOrCreateProcessActivityNode(entry *model.ProcessCacheEntry) *ProcessActivityNode {
	var node *ProcessActivityNode

	if entry == nil {
		return node
	}

	// look for a ProcessActivityNode by process cookie
	if entry.Cookie > 0 {
		var found bool
		node, found = ad.CookiesNode[entry.Cookie]
		if found {
			return node
		}
	}

	// find or create a ProcessActivityNode for the parent of the input ProcessCacheEntry. If the parent is a fork entry,
	// jump immediately to the next ancestor.
	parentNode := ad.FindOrCreateProcessActivityNode(entry.GetNextAncestorNoFork())

	// if parentNode is nil, the parent of the current node is out of tree (either because the parent is null, or it
	// doesn't match the dump tags).
	if parentNode == nil {

		// since the parent of the current entry wasn't inserted, we need to know if the current entry needs to be inserted.
		if !ad.Matches(entry) {
			return node
		}

		// go through the root nodes and check if one of them matches the input ProcessCacheEntry:
		for _, root := range ad.ProcessActivityTree {
			if root.Matches(entry, ad.differentiateArgs, ad.resolvers) {
				return root
			}
		}
		// if it doesn't, create a new ProcessActivityNode for the input ProcessCacheEntry
		node = NewProcessActivityNode(entry)
		// insert in the list of root entries
		ad.ProcessActivityTree = append(ad.ProcessActivityTree, node)

	} else {

		// if parentNode wasn't nil, then (at least) the parent is part of the activity dump. This means that we need
		// to add the current entry no matter if it matches the selector or not. Go through the root children of the
		// parent node and check if one of them matches the input ProcessCacheEntry.
		for _, child := range parentNode.Children {
			if child.Matches(entry, ad.differentiateArgs, ad.resolvers) {
				return child
			}
		}

		// if none of them matched, create a new ProcessActivityNode for the input processCacheEntry
		node = NewProcessActivityNode(entry)
		// insert in the list of root entries
		parentNode.Children = append(parentNode.Children, node)
	}

	// insert new cookie shortcut
	if entry.Cookie > 0 {
		ad.CookiesNode[entry.Cookie] = node
	}

	// set the pid of the input ProcessCacheEntry as traced
	_ = ad.tracedPIDs.Put(entry.Pid, uint64(0))

	return node
}

// Profile holds the list of rules generated from an activity dump
type Profile struct {
	Name     string
	Selector string
	Rules    []ProfileRule
}

// ProfileRule contains the data required to generate a rule
type ProfileRule struct {
	ID         string
	Expression string
}

// NewProfileRule returns a new ProfileRule
func NewProfileRule(expression string, ruleIDPrefix string) ProfileRule {
	return ProfileRule{
		ID:         ruleIDPrefix + "_" + utils.RandID(5),
		Expression: expression,
	}
}

func (ad *ActivityDump) generateFIMRules(file *FileActivityNode, activityNode *ProcessActivityNode, ancestors []*ProcessActivityNode, ruleIDPrefix string) []ProfileRule {
	var rules []ProfileRule

	if file.Open != nil {
		rule := NewProfileRule(fmt.Sprintf(
			"open.file.path == \"%s\" && open.file.in_upper_layer == %v && open.file.uid == %d && open.file.gid == %d && open.file.mode == %d",
			file.Open.File.PathnameStr,
			file.Open.File.InUpperLayer,
			file.Open.File.UID,
			file.Open.File.GID,
			file.Open.File.Mode),
			ruleIDPrefix,
		)
		rule.Expression += fmt.Sprintf(" && process.file.path == \"%s\"", activityNode.Process.PathnameStr)
		for _, parent := range ancestors {
			rule.Expression += fmt.Sprintf(" && process.ancestors.file.path == \"%s\"", parent.Process.PathnameStr)
		}
		rules = append(rules, rule)
	}

	for _, child := range file.Children {
		childrenRules := ad.generateFIMRules(child, activityNode, ancestors, ruleIDPrefix)
		rules = append(rules, childrenRules...)
	}

	return rules
}

func (ad *ActivityDump) generateRules(node *ProcessActivityNode, ancestors []*ProcessActivityNode, ruleIDPrefix string) []ProfileRule {
	var rules []ProfileRule

	// add exec rule
	rule := NewProfileRule(fmt.Sprintf(
		"exec.file.path == \"%s\" && process.uid == %d && process.gid == %d && process.cap_effective == %d && process.cap_permitted == %d",
		node.Process.PathnameStr,
		node.Process.UID,
		node.Process.GID,
		node.Process.CapEffective,
		node.Process.CapPermitted),
		ruleIDPrefix,
	)
	for _, parent := range ancestors {
		rule.Expression += fmt.Sprintf(" && process.ancestors.file.path == \"%s\"", parent.Process.PathnameStr)
	}
	rules = append(rules, rule)

	// add FIM rules
	for _, file := range node.Files {
		fimRules := ad.generateFIMRules(file, node, ancestors, ruleIDPrefix)
		rules = append(rules, fimRules...)
	}

	// add children rules recursively
	newAncestors := append([]*ProcessActivityNode{node}, ancestors...)
	for _, child := range node.Children {
		childrenRules := ad.generateRules(child, newAncestors, ruleIDPrefix)
		rules = append(rules, childrenRules...)
	}

	return rules
}

// GenerateProfileData generates a Profile from the activity dump
func (ad *ActivityDump) GenerateProfileData() Profile {
	p := Profile{
		Name: "profile_" + utils.RandID(5),
	}

	// generate selector
	if len(ad.Comm) > 0 {
		p.Selector = fmt.Sprintf("process.comm = \"%s\"", ad.Comm)
	}
	if len(ad.Tags) > 0 {
		if len(p.Selector) > 0 {
			p.Selector += " && "
		}
		for i, tag := range ad.Tags {
			if i >= 1 {
				p.Selector += " && "
			}
			p.Selector += fmt.Sprintf("\"%s\" in container.tags", tag)
		}
	}

	// Add rules
	for _, node := range ad.ProcessActivityTree {
		rules := ad.generateRules(node, nil, p.Name)
		p.Rules = append(p.Rules, rules...)
	}

	return p
}

// GetSelectorStr returns a string representation of the profile selector
func (ad *ActivityDump) GetSelectorStr() string {
	return fmt.Sprintf("tags: %s, comm: %s", strings.Join(ad.Tags, ", "), ad.Comm)
}

// SendStats sends activity dump stats
func (ad *ActivityDump) SendStats(client *statsd.Client) error {
	for evtType, count := range ad.processedCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType)}
		if value := atomic.LoadUint64(count); value > 0 {
			if err := client.Gauge(metrics.MetricActivityDumpProcessed, float64(value), tags, 1.0); err != nil {
				return errors.Wrapf(err, "couldn't send %s metric", metrics.MetricActivityDumpProcessed)
			}
		}
	}

	for evtType, count := range ad.addedCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType)}
		if value := atomic.LoadUint64(count); value > 0 {
			if err := client.Gauge(metrics.MetricActivityDumpAdded, float64(value), tags, 1.0); err != nil {
				return errors.Wrapf(err, "couldn't send %s metric", metrics.MetricActivityDumpAdded)
			}
		}
	}

	return nil
}

// Snapshot snapshots the processes in the activity dump to capture all the
func (ad *ActivityDump) Snapshot() error {
	for _, node := range ad.ProcessActivityTree {
		if err := node.snapshot(ad.resolvers, ad.scrubber); err != nil {
			return err
		}
		// iterate slowly
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// ProcessActivityNode holds the activity of a process
type ProcessActivityNode struct {
	Process model.Process `json:"process"`

	Files    []*FileActivityNode    `json:"files"`
	Children []*ProcessActivityNode `json:"children"`
}

// NewProcessActivityNode returns a new ProcessActivityNode instance
func NewProcessActivityNode(entry *model.ProcessCacheEntry) *ProcessActivityNode {
	pan := ProcessActivityNode{
		Process: entry.Process,
	}
	pan.retain()
	return &pan
}

// nolint: unused
func (pan *ProcessActivityNode) debug(prefix string) {
	fmt.Printf("%s- process: %s\n", prefix, pan.Process.PathnameStr)
	if len(pan.Files) > 0 {
		fmt.Printf("%s  files:\n", prefix)
		for _, f := range pan.Files {
			f.debug(fmt.Sprintf("%s\t-", prefix))
		}
	}
	if len(pan.Children) > 0 {
		fmt.Printf("%s  children:\n", prefix)
		for _, child := range pan.Children {
			child.debug(prefix + "\t")
		}
	}
}

func (pan *ProcessActivityNode) retain() {
	if pan.Process.ArgsEntry != nil && pan.Process.ArgsEntry.ArgsEnvsCacheEntry != nil {
		pan.Process.ArgsEntry.ArgsEnvsCacheEntry.Retain()
	}
	if pan.Process.EnvsEntry != nil && pan.Process.EnvsEntry.ArgsEnvsCacheEntry != nil {
		pan.Process.EnvsEntry.ArgsEnvsCacheEntry.Retain()
	}
}

func (pan *ProcessActivityNode) release() {
	if pan.Process.ArgsEntry != nil && pan.Process.ArgsEntry.ArgsEnvsCacheEntry != nil {
		pan.Process.ArgsEntry.ArgsEnvsCacheEntry.Release()
	}
	if pan.Process.EnvsEntry != nil && pan.Process.EnvsEntry.ArgsEnvsCacheEntry != nil {
		pan.Process.EnvsEntry.ArgsEnvsCacheEntry.Release()
	}
}

func (pan *ProcessActivityNode) recursiveRelease() {
	pan.release()
	for _, child := range pan.Children {
		child.recursiveRelease()
	}
}

// Matches return true if the process fields used to generate the dump are identical with the provided ProcessCacheEntry
func (pan *ProcessActivityNode) Matches(entry *model.ProcessCacheEntry, matchArgs bool, resolvers *Resolvers) bool {

	if pan.Process.Comm == entry.Comm && pan.Process.PathnameStr == entry.PathnameStr &&
		pan.Process.Credentials == entry.Credentials {

		if matchArgs {

			panArgs, _ := resolvers.ProcessResolver.GetProcessArgv(&pan.Process)
			entryArgs, _ := resolvers.ProcessResolver.GetProcessArgv(&entry.Process)
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

func extractFirstParent(path string) (string, int) {
	var prefix string
	var prefixLen int
	prefixes := strings.Split(path, "/")
	if len(prefixes) > 1 && len(prefixes[0]) == 0 {
		prefix = prefixes[1]
		prefixLen = len(prefix) + 1
	} else {
		prefix = prefixes[0]
		prefixLen = len(prefix)
	}
	return prefix, prefixLen
}

// InsertOpen inserts the provided file open event in the current node. This function returns true if a new entry was
// added, false if the event was dropped.
func (pan *ProcessActivityNode) InsertOpen(event *Event) bool {
	prefix, prefixLen := extractFirstParent(event.ResolveFilePath(&event.Open.File))
	if prefixLen == 0 {
		return false
	}

	for _, child := range pan.Files {
		if child.Name == prefix {
			return child.InsertOpen(&event.Open, event.Open.File.PathnameStr[prefixLen:])
		}
		// TODO: look for patterns / merge algo
	}

	// create new child
	if len(event.Open.File.PathnameStr) <= prefixLen+1 {
		pan.Files = append(pan.Files, NewFileActivityNode(&event.Open, prefix))
	} else {
		child := NewFileActivityNode(nil, prefix)
		child.InsertOpen(&event.Open, event.Open.File.PathnameStr[prefixLen:])
		pan.Files = append(pan.Files, child)
	}
	return true
}

// snapshot uses procfs to retrieve information about the current process
func (pan *ProcessActivityNode) snapshot(resolvers *Resolvers, scrubber *config.DataScrubber) error {
	// call snapshot for all the children of the current node
	for _, child := range pan.Children {
		if err := child.snapshot(resolvers, scrubber); err != nil {
			return err
		}
		// iterate slowly
		time.Sleep(50 * time.Millisecond)
	}

	// snapshot the current process
	p, err := process.NewProcess(int32(pan.Process.Pid))
	if err != nil {
		// the process doesn't exist anymore, ignore
		return nil
	}

	var files []string

	// list the files opened by the process
	fileFDs, err := p.OpenFiles()
	if err != nil {
		return err
	}

	for _, fd := range fileFDs {
		files = append(files, fd.Path)
	}

	// list the mmaped files of the process
	memoryMaps, err := p.MemoryMaps(false)
	if err != nil {
		return err
	}

	for _, mm := range *memoryMaps {
		if mm.Path != pan.Process.PathnameStr {
			files = append(files, mm.Path)
		}
	}

	// insert files
	for _, f := range files {
		if len(f) == 0 {
			continue
		}

		// fetch the file user, group and mode
		fullPath := filepath.Join(utils.RootPath(int32(pan.Process.Pid)), f)
		fileinfo, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		stat, ok := fileinfo.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		evt := NewEvent(resolvers, scrubber)

		evt.Open.Mode = stat.Mode
		evt.Open.File.PathnameStr = f
		evt.Open.File.BasenameStr = path.Base(f)
		evt.Open.File.FileFields.Mode = uint16(stat.Mode)
		evt.Open.File.FileFields.Inode = stat.Ino
		evt.Open.File.FileFields.UID = stat.Uid
		evt.Open.File.FileFields.GID = stat.Gid
		evt.Open.File.FileFields.MTime = uint64(resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec)))
		evt.Open.File.FileFields.CTime = uint64(resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)))
		pan.InsertOpen(evt)
	}
	return nil
}

// FileActivityNode holds a tree representation of a list of files
type FileActivityNode struct {
	Name string           `json:"name"`
	Open *model.OpenEvent `json:"open,omitempty"`

	Children []*FileActivityNode `json:"children"`
}

// NewFileActivityNode returns a new FileActivityNode instance
func NewFileActivityNode(event *model.OpenEvent, name string) *FileActivityNode {
	fan := &FileActivityNode{
		Name: name,
	}
	if event != nil {
		open := *event
		fan.Open = &open
	}
	return fan
}

// InsertOpen inserts an open event in a FileActivityNode. This function returns true if a new entry was added, false if
// the event was dropped.
func (fan *FileActivityNode) InsertOpen(event *model.OpenEvent, remainingPath string) bool {
	prefix, prefixLen := extractFirstParent(remainingPath)
	if prefixLen == 0 {
		return false
	}

	for _, child := range fan.Children {
		if child.Name == prefix {
			return child.InsertOpen(event, remainingPath[prefixLen:])
		}
		// TODO: look for patterns / merge algo
	}

	// create new child
	if len(remainingPath) <= prefixLen {
		fan.Children = append(fan.Children, NewFileActivityNode(event, prefix))
	} else {
		child := NewFileActivityNode(nil, prefix)
		child.InsertOpen(event, remainingPath[prefixLen:])
		fan.Children = append(fan.Children, child)
	}
	return true
}

// nolint: unused
func (fan *FileActivityNode) debug(prefix string) {
	fmt.Printf("%s %s\n", prefix, fan.Name)
	for _, child := range fan.Children {
		child.debug("\t" + prefix)
	}
}
