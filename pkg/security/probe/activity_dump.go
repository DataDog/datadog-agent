// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

//go:generate go run github.com/tinylib/msgp -o=activity_dump_gen_linux.go -tests=false

package probe

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/pkg/errors"
	"github.com/tinylib/msgp/msgp"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/activity_dump"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ActivityDumpStatus defines the state of an activity dump
type ActivityDumpStatus int

const (
	// Stopped means that the ActivityDump is not active
	Stopped ActivityDumpStatus = iota
	// Running means that the ActivityDump is active
	Running
)

// ActivityDump holds the activity tree for the workload defined by the provided list of tags
type ActivityDump struct {
	sync.Mutex         `msg:"-"`
	state              ActivityDumpStatus
	adm                *ActivityDumpManager
	processedCount     map[model.EventType]*uint64
	addedRuntimeCount  map[model.EventType]*uint64
	addedSnapshotCount map[model.EventType]*uint64

	CookiesNode         map[uint32]*ProcessActivityNode `msg:"-"`
	ProcessActivityTree []*ProcessActivityNode          `msg:"tree,omitempty"`
	DifferentiateArgs   bool                            `msg:"differentiate_args"`
	Comm                string                          `msg:"comm,omitempty"`
	ContainerID         string                          `msg:"container_id,omitempty"`
	Tags                []string                        `msg:"tags,omitempty"`
	Start               time.Time                       `msg:"start"`
	Timeout             time.Duration                   `msg:"-"`
	End                 time.Time                       `msg:"end"`
	timeoutRaw          int64                           `msg:"-"`
	Name                string                          `msg:"name"`

	StorageRequests map[activity_dump.StorageFormat][]activity_dump.StorageRequest `msg:"storage_requests,omitempty"`
}

// WithDumpOption can be used to configure an ActivityDump
//msgp:ignore WithDumpOption
type WithDumpOption func(ad *ActivityDump)

// NewActivityDump returns a new instance of an ActivityDump
func NewActivityDump(adm *ActivityDumpManager, options ...WithDumpOption) *ActivityDump {
	ad := ActivityDump{
		CookiesNode:        make(map[uint32]*ProcessActivityNode),
		Start:              time.Now(),
		adm:                adm,
		processedCount:     make(map[model.EventType]*uint64),
		addedRuntimeCount:  make(map[model.EventType]*uint64),
		addedSnapshotCount: make(map[model.EventType]*uint64),
		StorageRequests:    make(map[activity_dump.StorageFormat][]activity_dump.StorageRequest),
		Name:               fmt.Sprintf("activity-dump-%s", eval.RandString(10)),
	}

	for _, option := range options {
		option(&ad)
	}

	// generate counters
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		processed := uint64(0)
		runtime := uint64(0)
		snapshot := uint64(0)

		ad.processedCount[i] = &processed
		ad.addedRuntimeCount[i] = &runtime
		ad.addedSnapshotCount[i] = &snapshot
	}
	return &ad
}

// SetState sets the status of the activity dump
func (ad *ActivityDump) SetState(state ActivityDumpStatus) {
	ad.Lock()
	defer ad.Unlock()
	ad.state = state
}

// AddStorageRequest adds a storage request to an activity dump
func (ad *ActivityDump) AddStorageRequest(request activity_dump.StorageRequest) {
	ad.Lock()
	defer ad.Unlock()

	if ad.StorageRequests == nil {
		ad.StorageRequests = make(map[activity_dump.StorageFormat][]activity_dump.StorageRequest)
	}
	ad.StorageRequests[request.Format] = append(ad.StorageRequests[request.Format], request)
}

// getTimeoutRawTimestamp returns the timeout timestamp of the current activity dump as a monolitic kernel timestamp
func (ad *ActivityDump) getTimeoutRawTimestamp() int64 {
	if ad.timeoutRaw == 0 {
		ad.timeoutRaw = ad.adm.probe.resolvers.TimeResolver.ComputeMonotonicTimestamp(ad.Start.Add(ad.Timeout))
	}
	return ad.timeoutRaw
}

// updateTracedPidTimeout updates the timeout of a traced pid in kernel space
func (ad *ActivityDump) updateTracedPidTimeout(pid uint32) {
	// start by looking up any existing entry
	var timeout int64
	_ = ad.adm.tracedPIDsMap.Lookup(pid, &timeout)
	if timeout < ad.getTimeoutRawTimestamp() {
		_ = ad.adm.tracedPIDsMap.Put(pid, ad.getTimeoutRawTimestamp())
	}
}

// commMatches returns true if the ActivityDump comm matches the provided comm
func (ad *ActivityDump) commMatches(comm string) bool {
	return ad.Comm == comm
}

// containerIDMatches returns true if the ActivityDump container ID matches the provided container ID
func (ad *ActivityDump) containerIDMatches(containerID string) bool {
	return ad.ContainerID == containerID
}

// Matches returns true if the provided list of tags and / or the provided comm match the current ActivityDump
func (ad *ActivityDump) Matches(entry *model.ProcessCacheEntry) bool {
	if entry == nil {
		return false
	}

	if len(ad.ContainerID) > 0 {
		if !ad.containerIDMatches(entry.ContainerID) {
			return false
		}
	}

	if len(ad.Comm) > 0 {
		if !ad.commMatches(entry.Comm) {
			return false
		}
	}

	return true
}

// Stop stops an active dump
func (ad *ActivityDump) Stop() {
	ad.Lock()
	defer ad.Unlock()
	ad.state = Stopped
	ad.End = time.Now()

	// remove comm from kernel space
	if len(ad.Comm) > 0 {
		commB := make([]byte, 16)
		copy(commB, ad.Comm)
		err := ad.adm.tracedCommsMap.Delete(commB)
		if err != nil {
			seclog.Debugf("couldn't delete activity dump filter comm(%s): %v", ad.Comm, err)
		}
	}

	// remove container ID from kernel space
	if len(ad.ContainerID) > 0 {
		containerIDB := make([]byte, model.ContainerIDLen)
		copy(containerIDB, ad.ContainerID)
		err := ad.adm.tracedCgroupsMap.Delete(containerIDB)
		if err != nil {
			seclog.Debugf("couldn't delete activity dump filter containerID(%s): %v", ad.ContainerID, err)
		}
	}
}

// Release releases all the resources held by an activity dump
func (ad *ActivityDump) Release() {
	ad.Lock()
	defer ad.Unlock()

	// release all shared resources
	for _, p := range ad.ProcessActivityTree {
		p.recursiveRelease()
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
	ad.Lock()
	defer ad.Unlock()

	if ad.state != Running {
		// this activity dump is not running anymore, ignore event
		return false
	}

	// ignore fork events for now
	if event.GetEventType() == model.ForkEventType {
		return false
	}

	// metrics
	defer func() {
		if newEntry {
			// this doesn't count the exec events which are counted separately
			atomic.AddUint64(ad.addedRuntimeCount[event.GetEventType()], 1)
		}
	}()

	// find the node where the event should be inserted
	node := ad.findOrCreateProcessActivityNode(event.ResolveProcessCacheEntry(), Runtime)
	if node == nil {
		// a process node couldn't be found for the provided event as it doesn't match the ActivityDump query
		return false
	}

	// check if this event type is traced
	var traced bool
	for _, evtType := range ad.adm.probe.config.ActivityDumpTracedEventTypes {
		if evtType == event.GetEventType() {
			traced = true
		}
	}
	if !traced {
		return false
	}

	// resolve fields
	event.ResolveFields()

	// the count of processed events is the count of events that matched the activity dump selector = the events for
	// which we successfully found a process activity node
	atomic.AddUint64(ad.processedCount[event.GetEventType()], 1)

	// insert the event based on its type
	switch event.GetEventType() {
	case model.FileOpenEventType:
		return node.InsertFileEvent(&event.Open.File, event, Runtime)
	case model.DNSEventType:
		return node.InsertDNSEvent(&event.DNS)
	}
	return false
}

// findOrCreateProcessActivityNode finds or a create a new process activity node in the activity dump if the entry
// matches the activity dump selector.
func (ad *ActivityDump) findOrCreateProcessActivityNode(entry *model.ProcessCacheEntry, generationType NodeGenerationType) *ProcessActivityNode {
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
	parentNode := ad.findOrCreateProcessActivityNode(entry.GetNextAncestorNoFork(), Snapshot)

	// if parentNode is nil, the parent of the current node is out of tree (either because the parent is null, or it
	// doesn't match the dump tags).
	if parentNode == nil {

		// since the parent of the current entry wasn't inserted, we need to know if the current entry needs to be inserted.
		if !ad.Matches(entry) {
			return node
		}

		// go through the root nodes and check if one of them matches the input ProcessCacheEntry:
		for _, root := range ad.ProcessActivityTree {
			if root.Matches(entry, ad.DifferentiateArgs, ad.adm.probe.resolvers) {
				return root
			}
		}
		// if it doesn't, create a new ProcessActivityNode for the input ProcessCacheEntry
		node = NewProcessActivityNode(entry, generationType)
		// insert in the list of root entries
		ad.ProcessActivityTree = append(ad.ProcessActivityTree, node)

	} else {

		// if parentNode wasn't nil, then (at least) the parent is part of the activity dump. This means that we need
		// to add the current entry no matter if it matches the selector or not. Go through the root children of the
		// parent node and check if one of them matches the input ProcessCacheEntry.
		for _, child := range parentNode.Children {
			if child.Matches(entry, ad.DifferentiateArgs, ad.adm.probe.resolvers) {
				return child
			}
		}

		// if none of them matched, create a new ProcessActivityNode for the input processCacheEntry
		node = NewProcessActivityNode(entry, generationType)
		// insert in the list of root entries
		parentNode.Children = append(parentNode.Children, node)
	}

	// insert new cookie shortcut
	if entry.Cookie > 0 {
		ad.CookiesNode[entry.Cookie] = node
	}

	// count new entry
	switch generationType {
	case Runtime:
		atomic.AddUint64(ad.addedRuntimeCount[model.ExecEventType], 1)
	case Snapshot:
		atomic.AddUint64(ad.addedSnapshotCount[model.ExecEventType], 1)
	}

	// set the pid of the input ProcessCacheEntry as traced
	ad.updateTracedPidTimeout(entry.Pid)

	return node
}

// GetSelectorStr returns a string representation of the profile selector
func (ad *ActivityDump) GetSelectorStr() string {
	if len(ad.Tags) > 0 {
		return strings.Join(ad.Tags, ",")
	}
	if len(ad.ContainerID) > 0 {
		return fmt.Sprintf("container_id:%s", ad.ContainerID)
	}
	if len(ad.Comm) > 0 {
		return fmt.Sprintf("comm:%s", ad.Comm)
	}
	return "empty_selector"
}

// SendStats sends activity dump stats
func (ad *ActivityDump) SendStats() error {
	for evtType, count := range ad.processedCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType)}
		if value := atomic.SwapUint64(count, 0); value > 0 {
			if err := ad.adm.probe.statsdClient.Count(metrics.MetricActivityDumpEventProcessed, int64(value), tags, 1.0); err != nil {
				return errors.Wrapf(err, "couldn't send %s metric", metrics.MetricActivityDumpEventProcessed)
			}
		}
	}

	for evtType, count := range ad.addedRuntimeCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("generation_type:%s", Runtime)}
		if value := atomic.SwapUint64(count, 0); value > 0 {
			if err := ad.adm.probe.statsdClient.Count(metrics.MetricActivityDumpEventAdded, int64(value), tags, 1.0); err != nil {
				return errors.Wrapf(err, "couldn't send %s metric", metrics.MetricActivityDumpEventAdded)
			}
		}
	}

	for evtType, count := range ad.addedSnapshotCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("generation_type:%s", Snapshot)}
		if value := atomic.SwapUint64(count, 0); value > 0 {
			if err := ad.adm.probe.statsdClient.Count(metrics.MetricActivityDumpEventAdded, int64(value), tags, 1.0); err != nil {
				return errors.Wrapf(err, "couldn't send %s metric", metrics.MetricActivityDumpEventAdded)
			}
		}
	}

	return nil
}

// Snapshot snapshots the processes in the activity dump to capture all the
func (ad *ActivityDump) Snapshot() error {
	ad.Lock()
	defer ad.Unlock()

	for _, pan := range ad.ProcessActivityTree {
		if err := pan.snapshot(ad); err != nil {
			return err
		}
		// iterate slowly
		time.Sleep(50 * time.Millisecond)
	}

	// try to resolve the tags now
	_ = ad.resolveTags()
	return nil
}

// ResolveTags tries to resolve the activity dump tags
func (ad *ActivityDump) ResolveTags() error {
	ad.Lock()
	defer ad.Unlock()
	return ad.resolveTags()
}

// resolveTags thread unsafe version ot ResolveTags
func (ad *ActivityDump) resolveTags() error {
	if len(ad.Tags) > 0 || len(ad.ContainerID) == 0 {
		return nil
	}

	var err error
	ad.Tags, err = ad.adm.probe.resolvers.TagsResolver.ResolveWithErr(ad.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", ad.ContainerID, err)
	}
	return nil
}

// ToSecurityActivityDumpMessage returns a pointer to a SecurityActivityDumpMessage
func (ad *ActivityDump) ToSecurityActivityDumpMessage() *api.SecurityActivityDumpMessage {
	var storage []*api.StorageRequestMessage
	for _, requests := range ad.StorageRequests {
		for _, request := range requests {
			storage = append(storage, request.ToStorageRequestMessage(ad.Name))
		}
	}

	return &api.SecurityActivityDumpMessage{
		Comm:              ad.Comm,
		ContainerID:       ad.ContainerID,
		Tags:              ad.Tags,
		DifferentiateArgs: ad.DifferentiateArgs,
		Timeout:           ad.Timeout.String(),
		Start:             ad.Start.String(),
		Left:              ad.Start.Add(ad.Timeout).Sub(time.Now()).String(),
		Storage:           storage,
		Name:              ad.Name,
	}
}

// ToTranscodingRequestMessage returns a pointer to a TranscodingRequestMessage
func (ad *ActivityDump) ToTranscodingRequestMessage() *api.TranscodingRequestMessage {
	var storage []*api.StorageRequestMessage
	for _, requests := range ad.StorageRequests {
		for _, request := range requests {
			storage = append(storage, request.ToStorageRequestMessage(ad.Name))
		}
	}

	return &api.TranscodingRequestMessage{
		Storage: storage,
	}
}

// Encode encodes an activity dump in the provided format
func (ad *ActivityDump) Encode(format activity_dump.StorageFormat) (*bytes.Buffer, error) {
	switch format {
	case activity_dump.JSON:
		return ad.EncodeJSON()
	case activity_dump.MSGP:
		return ad.EncodeMSGP()
	case activity_dump.DOT:
		return ad.EncodeDOT()
	case activity_dump.Profile:
		return ad.EncodeProfile()
	default:
		return nil, fmt.Errorf("couldn't encode activity dump [%s] as [%s]: unknown format", ad.GetSelectorStr(), format)
	}
}

// EncodeJSON encodes an activity dump in the JSON format
func (ad *ActivityDump) EncodeJSON() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	msgpRaw, err := ad.MarshalMsg(nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't encode in %s: %v\n", activity_dump.MSGP, err)
	}
	raw := bytes.NewBuffer(nil)
	_, err = msgp.UnmarshalAsJSON(raw, msgpRaw)
	if err != nil {
		return nil, fmt.Errorf("couldn't encode %s: %v\n", activity_dump.JSON, err)
	}
	return raw, nil
}

// EncodeMSGP encodes an activity dump in the MSGP format
func (ad *ActivityDump) EncodeMSGP() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	raw, err := ad.MarshalMsg(nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't encode in %s: %v\n", activity_dump.MSGP, err)
	}
	return bytes.NewBuffer(raw), nil
}

// Unzip decompresses a compressed input file
func (ad *ActivityDump) Unzip(inputFile string, ext string) (string, error) {
	// uncompress the file first
	f, err := os.Open(inputFile)
	if err != nil {
		return "", fmt.Errorf("couldn't open input file: %w", err)
	}

	seclog.Infof("unzipping %s", inputFile)
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return "", fmt.Errorf("couldn't create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	outputFile, err := os.Create(strings.TrimSuffix(inputFile, ext))
	if err != nil {
		f.Close()
		return "", fmt.Errorf("couldn't create gzip output file: %w", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, gzipReader)
	if err != nil {
		return "", fmt.Errorf("couldn't unzip %s: %w", inputFile, err)
	}
	return strings.TrimSuffix(inputFile, ext), nil
}

// Decode decodes an activity dump from a file
func (ad *ActivityDump) Decode(inputFile string) error {
	var err error
	ext := filepath.Ext(inputFile)

	if ext == ".gz" {
		inputFile, err = ad.Unzip(inputFile, ext)
		if err != nil {
			return err
		}
		ext = filepath.Ext(inputFile)
	}

	format, err := activity_dump.ParseStorageFormat(ext)
	if err != nil {
		return err
	}
	switch format {
	case activity_dump.MSGP:
		return ad.DecodeMSGP(inputFile)
	default:
		return fmt.Errorf("unsupported input format: %s", format)
	}
}

// DecodeMSGP decodes an activity dump as MSGP
func (ad *ActivityDump) DecodeMSGP(inputFile string) error {
	ad.Lock()
	defer ad.Unlock()

	f, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("couldn't open activity dump file: %w", err)
	}
	defer f.Close()

	msgpReader := msgp.NewReader(f)
	err = ad.DecodeMsg(msgpReader)
	if err != nil {
		return fmt.Errorf("couldn't parse activity dump file: %w", err)
	}
	return nil
}

// ProcessActivityNode holds the activity of a process
type ProcessActivityNode struct {
	id             string
	Process        model.Process      `msg:"process"`
	GenerationType NodeGenerationType `msg:"generation_type"`

	Files    map[string]*FileActivityNode `msg:"files,omitempty"`
	DNSNames map[string]*DNSNode          `msg:"dns,omitempty"`
	Children []*ProcessActivityNode       `msg:"children,omitempty"`
}

// GetID returns a unique ID to identify the current node
func (pan *ProcessActivityNode) GetID() string {
	if len(pan.id) == 0 {
		pan.id = eval.RandString(5)
	}
	return pan.id
}

// NewProcessActivityNode returns a new ProcessActivityNode instance
func NewProcessActivityNode(entry *model.ProcessCacheEntry, generationType NodeGenerationType) *ProcessActivityNode {
	pan := ProcessActivityNode{
		Process:        entry.Process,
		GenerationType: generationType,
		Files:          make(map[string]*FileActivityNode),
		DNSNames:       make(map[string]*DNSNode),
	}
	_ = pan.GetID()
	pan.retain()
	return &pan
}

// nolint: unused
func (pan *ProcessActivityNode) debug(prefix string) {
	fmt.Printf("%s- process: %s\n", prefix, pan.Process.FileEvent.PathnameStr)
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

	if pan.Process.Comm == entry.Comm && pan.Process.FileEvent.PathnameStr == entry.FileEvent.PathnameStr &&
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
	if len(path) == 0 {
		return "", 0
	}
	if path == "/" {
		return "", 0
	}

	var add int
	if path[0] == '/' {
		path = path[1:]
		add = 1
	}

	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			return path[0:i], i + add
		}
	}

	return path, len(path) + add
}

// InsertFileEvent inserts the provided file event in the current node. This function returns true if a new entry was
// added, false if the event was dropped.
func (pan *ProcessActivityNode) InsertFileEvent(fileEvent *model.FileEvent, event *Event, generationType NodeGenerationType) bool {
	parent, nextParentIndex := extractFirstParent(event.ResolveFilePath(fileEvent))
	if nextParentIndex == 0 {
		return false
	}

	// TODO: look for patterns / merge algo

	child, ok := pan.Files[parent]
	if ok {
		return child.InsertFileEvent(fileEvent, event, fileEvent.PathnameStr[nextParentIndex:], generationType)
	}

	// create new child
	if len(fileEvent.PathnameStr) <= nextParentIndex+1 {
		pan.Files[parent] = NewFileActivityNode(fileEvent, event, parent, generationType)
	} else {
		child := NewFileActivityNode(nil, nil, parent, generationType)
		child.InsertFileEvent(fileEvent, event, fileEvent.PathnameStr[nextParentIndex:], generationType)
		pan.Files[parent] = child
	}
	return true
}

// snapshot uses procfs to retrieve information about the current process
func (pan *ProcessActivityNode) snapshot(ad *ActivityDump) error {
	// call snapshot for all the children of the current node
	for _, child := range pan.Children {
		if err := child.snapshot(ad); err != nil {
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

	for _, eventType := range ad.adm.probe.config.ActivityDumpTracedEventTypes {
		switch eventType {
		case model.FileOpenEventType:
			if err = pan.snapshotFiles(p, ad); err != nil {
				return err
			}
		}
	}
	return nil
}

func (pan *ProcessActivityNode) snapshotFiles(p *process.Process, ad *ActivityDump) error {
	// list the files opened by the process
	fileFDs, err := p.OpenFiles()
	if err != nil {
		return err
	}

	var files []string
	for _, fd := range fileFDs {
		files = append(files, fd.Path)
	}

	// list the mmaped files of the process
	memoryMaps, err := p.MemoryMaps(false)
	if err != nil {
		return err
	}

	for _, mm := range *memoryMaps {
		if mm.Path != pan.Process.FileEvent.PathnameStr {
			files = append(files, mm.Path)
		}
	}

	// insert files
	var fileinfo os.FileInfo
	var resolvedPath string
	for _, f := range files {
		if len(f) == 0 {
			continue
		}

		// fetch the file user, group and mode
		fullPath := filepath.Join(utils.RootPath(int32(pan.Process.Pid)), f)
		fileinfo, err = os.Stat(fullPath)
		if err != nil {
			continue
		}
		stat, ok := fileinfo.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}

		evt := NewEvent(ad.adm.probe.resolvers, ad.adm.probe.scrubber, ad.adm.probe)
		evt.Event.Type = uint32(model.FileOpenEventType)

		resolvedPath, err = filepath.EvalSymlinks(f)
		if err != nil {
			evt.Open.File.PathnameStr = resolvedPath
		} else {
			evt.Open.File.PathnameStr = f
		}
		evt.Open.File.BasenameStr = path.Base(evt.Open.File.PathnameStr)
		evt.Open.File.FileFields.Mode = uint16(stat.Mode)
		evt.Open.File.FileFields.Inode = stat.Ino
		evt.Open.File.FileFields.UID = stat.Uid
		evt.Open.File.FileFields.GID = stat.Gid
		evt.Open.File.FileFields.MTime = uint64(ad.adm.probe.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec)))
		evt.Open.File.FileFields.CTime = uint64(ad.adm.probe.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)))

		evt.Open.File.Mode = evt.Open.File.FileFields.Mode
		// TODO: add open flags by parsing `/proc/[pid]/fdinfo/fd` + O_RDONLY|O_CLOEXEC for the shared libs

		if pan.InsertFileEvent(&evt.Open.File, evt, Snapshot) {
			// count this new entry
			atomic.AddUint64(ad.addedSnapshotCount[model.FileOpenEventType], 1)
		}
	}
	return nil
}

// InsertDNSEvent inserts
func (pan *ProcessActivityNode) InsertDNSEvent(evt *model.DNSEvent) bool {
	if dnsNode, ok := pan.DNSNames[evt.Name]; ok {
		// look for the DNS request type
		for _, req := range dnsNode.requests {
			if req.Type == evt.Type {
				return false
			}
		}

		// insert the new request
		dnsNode.requests = append(dnsNode.requests, *evt)
		return true
	}
	pan.DNSNames[evt.Name] = NewDNSNode(evt)
	return true
}

// FileActivityNode holds a tree representation of a list of files
type FileActivityNode struct {
	id             string
	Name           string             `msg:"name"`
	File           *model.FileEvent   `msg:"file,omitempty"`
	GenerationType NodeGenerationType `msg:"generation_type"`
	FirstSeen      time.Time          `msg:"first_seen,omitempty"`

	Open *OpenNode `msg:"open,omitempty"`

	Children map[string]*FileActivityNode `msg:"children,omitempty"`
}

// GetID returns a unique ID to identify the current node
func (fan *FileActivityNode) GetID() string {
	if len(fan.id) == 0 {
		fan.id = eval.RandString(5)
	}
	return fan.id
}

// OpenNode contains the relevant fields of an Open event on which we might want to write a profiling rule
type OpenNode struct {
	model.SyscallEvent
	Flags uint32 `msg:"flags"`
	Mode  uint32 `msg:"mode"`
}

// NewFileActivityNode returns a new FileActivityNode instance
func NewFileActivityNode(fileEvent *model.FileEvent, event *Event, name string, generationType NodeGenerationType) *FileActivityNode {
	fan := &FileActivityNode{
		Name:           name,
		GenerationType: generationType,
		Children:       make(map[string]*FileActivityNode),
	}
	_ = fan.GetID()
	if fileEvent != nil {
		fileEventTmp := *fileEvent
		fan.File = &fileEventTmp
	}
	fan.enrichFromEvent(event)
	return fan
}

func (fan *FileActivityNode) getNodeLabel() string {
	label := fan.Name
	if fan.Open != nil {
		label += " [open]"
	}
	return label
}

func (fan *FileActivityNode) enrichFromEvent(event *Event) {
	if event == nil {
		return
	}
	if fan.FirstSeen.IsZero() {
		fan.FirstSeen = event.ResolveEventTimestamp()
	}

	switch event.GetEventType() {
	case model.FileOpenEventType:
		fan.Open = &OpenNode{
			SyscallEvent: event.Open.SyscallEvent,
			Flags:        event.Open.Flags,
			Mode:         event.Open.Mode,
		}
	}
}

// InsertFileEvent inserts an event in a FileActivityNode. This function returns true if a new entry was added, false if
// the event was dropped.
func (fan *FileActivityNode) InsertFileEvent(fileEvent *model.FileEvent, event *Event, remainingPath string, generationType NodeGenerationType) bool {
	parent, nextParentIndex := extractFirstParent(remainingPath)
	if nextParentIndex == 0 {
		fan.enrichFromEvent(event)
		return false
	}

	// TODO: look for patterns / merge algo

	child, ok := fan.Children[parent]
	if ok {
		return child.InsertFileEvent(fileEvent, event, remainingPath[nextParentIndex:], generationType)
	}

	// create new child
	if len(remainingPath) <= nextParentIndex+1 {
		fan.Children[parent] = NewFileActivityNode(fileEvent, event, parent, generationType)
	} else {
		child := NewFileActivityNode(nil, nil, parent, generationType)
		child.InsertFileEvent(fileEvent, event, remainingPath[nextParentIndex:], generationType)
		fan.Children[parent] = child
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

// DNSNode is used to store a DNS node
type DNSNode struct {
	requests []model.DNSEvent `msg:"requests"`
	id       string
}

// NewDNSNode returns a new DNSNode instance
func NewDNSNode(event *model.DNSEvent) *DNSNode {
	return &DNSNode{
		requests: []model.DNSEvent{*event},
	}
}

// GetID returns the ID of the current DNS node
func (n *DNSNode) GetID() string {
	if len(n.id) == 0 {
		n.id = eval.RandString(5)
	}
	return n.id
}
