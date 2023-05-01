// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

package dump

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/cilium/ebpf"
	"github.com/prometheus/procfs"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/encoding/protojson"

	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	stime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// ProtobufVersion defines the protobuf version in use
	ProtobufVersion = "v1"
	// ActivityDumpSource defines the source of activity dumps
	ActivityDumpSource = "runtime-security-agent"
)

// ActivityDumpStatus defines the state of an activity dump
type ActivityDumpStatus int

const (
	// Stopped means that the ActivityDump is not active
	Stopped ActivityDumpStatus = iota
	// Disabled means that the ActivityDump is ready to be in running state, but we're missing the kernel space filters
	// to start retrieving events from kernel space
	Disabled
	// Paused means that the ActivityDump is ready to be in running state, but the kernel space filters have been configured
	// to prevent from being sent over the perf map
	Paused
	// Running means that the ActivityDump is active
	Running
)

// Metadata is used to provide context about the activity dump
type Metadata struct {
	AgentVersion      string `json:"agent_version"`
	AgentCommit       string `json:"agent_commit"`
	KernelVersion     string `json:"kernel_version"`
	LinuxDistribution string `json:"linux_distribution"`
	Arch              string `json:"arch"`

	Name              string    `json:"name"`
	ProtobufVersion   string    `json:"protobuf_version"`
	DifferentiateArgs bool      `json:"differentiate_args"`
	Comm              string    `json:"comm,omitempty"`
	ContainerID       string    `json:"-"`
	Start             time.Time `json:"start"`
	End               time.Time `json:"end"`
	Size              uint64    `json:"activity_dump_size,omitempty"`
	Serialization     string    `json:"serialization,omitempty"`
}

// ActivityDump holds the activity tree for the workload defined by the provided list of tags. The encoding described by
// the `msg` annotation is used to generate the activity dump file while the encoding described by the `json` annotation
// is used to generate the activity dump metadata sent to the event platform.
// easyjson:json
type ActivityDump struct {
	*sync.Mutex
	state              ActivityDumpStatus
	adm                *ActivityDumpManager
	processedCount     map[model.EventType]*atomic.Uint64
	addedRuntimeCount  map[model.EventType]*atomic.Uint64
	addedSnapshotCount map[model.EventType]*atomic.Uint64
	eventTypeDrop      map[model.EventType]*atomic.Uint64
	brokenLineageDrop  *atomic.Uint64
	validRootNodeDrop  *atomic.Uint64
	bindFamilyDrop     *atomic.Uint64

	countedByLimiter bool

	shouldMergePaths bool
	pathMergedCount  *atomic.Uint64
	nodeStats        ActivityDumpNodeStats

	// standard attributes used by the intake
	Host    string   `json:"host,omitempty"`
	Service string   `json:"service,omitempty"`
	Source  string   `json:"ddsource,omitempty"`
	Tags    []string `json:"-"`
	DDTags  string   `json:"ddtags,omitempty"`

	CookiesNode         map[uint32]*ProcessActivityNode                  `json:"-"`
	ProcessActivityTree []*ProcessActivityNode                           `json:"-"`
	StorageRequests     map[config.StorageFormat][]config.StorageRequest `json:"-"`

	// Dump metadata
	Metadata

	// Used to store the global list of DNS names contained in this dump
	// this is a hack used to provide this global list to the backend in the JSON header
	// instead of in the protobuf payload.
	DNSNames *utils.StringKeys `json:"dns_names"`

	// Load config
	LoadConfig       *model.ActivityDumpLoadConfig `json:"-"`
	LoadConfigCookie uint32                        `json:"-"`
}

// NewActivityDumpLoadConfig returns a new instance of ActivityDumpLoadConfig
func NewActivityDumpLoadConfig(evt []model.EventType, timeout time.Duration, waitListTimeout time.Duration, rate int, start time.Time, resolver *stime.Resolver) *model.ActivityDumpLoadConfig {
	adlc := &model.ActivityDumpLoadConfig{
		TracedEventTypes: evt,
		Timeout:          timeout,
		Rate:             uint32(rate),
	}
	if resolver != nil {
		adlc.StartTimestampRaw = uint64(resolver.ComputeMonotonicTimestamp(start))
		adlc.EndTimestampRaw = uint64(resolver.ComputeMonotonicTimestamp(start.Add(timeout)))
		adlc.WaitListTimestampRaw = uint64(resolver.ComputeMonotonicTimestamp(start.Add(waitListTimeout)))
	}
	return adlc
}

// NewEmptyActivityDump returns a new zero-like instance of an ActivityDump
func NewEmptyActivityDump() *ActivityDump {
	ad := &ActivityDump{
		Mutex:              &sync.Mutex{},
		CookiesNode:        make(map[uint32]*ProcessActivityNode),
		processedCount:     make(map[model.EventType]*atomic.Uint64),
		addedRuntimeCount:  make(map[model.EventType]*atomic.Uint64),
		addedSnapshotCount: make(map[model.EventType]*atomic.Uint64),
		eventTypeDrop:      make(map[model.EventType]*atomic.Uint64),
		brokenLineageDrop:  atomic.NewUint64(0),
		validRootNodeDrop:  atomic.NewUint64(0),
		bindFamilyDrop:     atomic.NewUint64(0),
		pathMergedCount:    atomic.NewUint64(0),
		StorageRequests:    make(map[config.StorageFormat][]config.StorageRequest),

		DNSNames: utils.NewStringKeys(nil),
	}

	// generate counters
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		ad.processedCount[i] = atomic.NewUint64(0)
		ad.addedRuntimeCount[i] = atomic.NewUint64(0)
		ad.addedSnapshotCount[i] = atomic.NewUint64(0)
		ad.eventTypeDrop[i] = atomic.NewUint64(0)
	}
	return ad
}

// WithDumpOption can be used to configure an ActivityDump
type WithDumpOption func(ad *ActivityDump)

// NewActivityDump returns a new instance of an ActivityDump
func NewActivityDump(adm *ActivityDumpManager, options ...WithDumpOption) *ActivityDump {
	ad := NewEmptyActivityDump()
	now := time.Now()
	ad.Metadata = Metadata{
		AgentVersion:      version.AgentVersion,
		AgentCommit:       version.Commit,
		KernelVersion:     adm.kernelVersion.Code.String(),
		LinuxDistribution: adm.kernelVersion.OsRelease["PRETTY_NAME"],
		Name:              fmt.Sprintf("activity-dump-%s", utils.RandString(10)),
		ProtobufVersion:   ProtobufVersion,
		Start:             now,
		End:               now.Add(adm.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout),
		Arch:              probes.RuntimeArch,
	}
	ad.Host = adm.hostname
	ad.Source = ActivityDumpSource
	ad.adm = adm
	ad.shouldMergePaths = adm.config.RuntimeSecurity.ActivityDumpPathMergeEnabled

	// set load configuration to initial defaults
	ad.LoadConfig = NewActivityDumpLoadConfig(
		adm.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		adm.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout,
		adm.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		adm.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
		adm.timeResolver,
	)
	ad.LoadConfigCookie = utils.NewCookie()

	for _, option := range options {
		option(ad)
	}
	return ad
}

// NewActivityDumpFromMessage returns a new ActivityDump from a SecurityActivityDumpMessage.
func NewActivityDumpFromMessage(msg *api.ActivityDumpMessage) (*ActivityDump, error) {
	metadata := msg.GetMetadata()
	if metadata == nil {
		return nil, fmt.Errorf("couldn't create new ActivityDump: missing activity dump metadata")
	}

	startTime, err := time.Parse(time.RFC822, metadata.GetStart())
	if err != nil {
		return nil, fmt.Errorf("couldn't parse start time [%s]: %w", metadata.GetStart(), err)
	}
	timeout, err := time.ParseDuration(metadata.GetTimeout())
	if err != nil {
		return nil, fmt.Errorf("couldn't parse timeout [%s]: %w", metadata.GetTimeout(), err)
	}

	ad := NewEmptyActivityDump()
	ad.Host = msg.GetHost()
	ad.Service = msg.GetService()
	ad.Source = msg.GetSource()
	ad.Tags = msg.GetTags()
	ad.Metadata = Metadata{
		AgentVersion:      metadata.GetAgentVersion(),
		AgentCommit:       metadata.GetAgentCommit(),
		KernelVersion:     metadata.GetKernelVersion(),
		LinuxDistribution: metadata.GetLinuxDistribution(),
		Name:              metadata.GetName(),
		ProtobufVersion:   metadata.GetProtobufVersion(),
		DifferentiateArgs: metadata.GetDifferentiateArgs(),
		Comm:              metadata.GetComm(),
		ContainerID:       metadata.GetContainerID(),
		Start:             startTime,
		End:               startTime.Add(timeout),
		Size:              metadata.GetSize(),
		Arch:              metadata.GetArch(),
	}
	ad.LoadConfig = NewActivityDumpLoadConfig(
		[]model.EventType{},
		timeout,
		0,
		0,
		startTime,
		nil,
	)
	ad.DNSNames = utils.NewStringKeys(msg.GetDNSNames())

	// parse requests from message
	for _, request := range msg.GetStorage() {
		storageType, err := config.ParseStorageType(request.GetType())
		if err != nil {
			// invalid storage type, ignore
			continue
		}
		storageFormat, err := config.ParseStorageFormat(request.GetFormat())
		if err != nil {
			// invalid storage format, ignore
			continue
		}
		ad.StorageRequests[storageFormat] = append(ad.StorageRequests[storageFormat], config.NewStorageRequest(
			storageType,
			storageFormat,
			request.GetCompression(),
			filepath.Base(request.File),
		))
	}
	return ad, nil
}

// computeSyscallsList returns the aggregated list of all syscalls
func (ad *ActivityDump) computeSyscallsList() []uint32 {
	mask := make(map[int]uint32)
	var nodes []*ProcessActivityNode
	var node *ProcessActivityNode
	if len(ad.ProcessActivityTree) > 0 {
		node = ad.ProcessActivityTree[0]
		nodes = ad.ProcessActivityTree[1:]
	}

	for node != nil {
		for _, nr := range node.Syscalls {
			mask[nr] = 1
		}
		for _, child := range node.Children {
			nodes = append(nodes, child)
		}
		if len(nodes) > 0 {
			node = nodes[0]
			nodes = nodes[1:]
		} else {
			node = nil
		}
	}

	output := make([]uint32, 0, len(mask))
	for key := range mask {
		output = append(output, uint32(key))
	}
	return output
}

// SetState sets the status of the activity dump
func (ad *ActivityDump) SetState(state ActivityDumpStatus) {
	ad.Lock()
	defer ad.Unlock()
	ad.state = state
}

// AddStorageRequest adds a storage request to an activity dump
func (ad *ActivityDump) AddStorageRequest(request config.StorageRequest) {
	ad.Lock()
	defer ad.Unlock()

	if ad.StorageRequests == nil {
		ad.StorageRequests = make(map[config.StorageFormat][]config.StorageRequest)
	}
	ad.StorageRequests[request.Format] = append(ad.StorageRequests[request.Format], request)
}

func (ad *ActivityDump) checkInMemorySize() {
	if ad.computeInMemorySize() < int64(ad.adm.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
		return
	}

	// pause the dump so that we no longer retrieve events from kernel space, the serialization will be handled later by
	// the load controller
	if err := ad.pause(); err != nil {
		seclog.Errorf("couldn't pause dump: %v", err)
	}
}

// ComputeInMemorySize returns the size of a dump in memory
func (ad *ActivityDump) ComputeInMemorySize() int64 {
	ad.Lock()
	defer ad.Unlock()
	return ad.computeInMemorySize()
}

// computeInMemorySize thread unsafe version of ComputeInMemorySize
func (ad *ActivityDump) computeInMemorySize() int64 {
	return ad.nodeStats.approximateSize()
}

// SetLoadConfig set the load config of the current activity dump
func (ad *ActivityDump) SetLoadConfig(cookie uint32, config model.ActivityDumpLoadConfig) {
	ad.LoadConfig = &config
	ad.LoadConfigCookie = cookie

	// Update metadata
	ad.Metadata.Start = ad.adm.timeResolver.ResolveMonotonicTimestamp(ad.LoadConfig.StartTimestampRaw)
	ad.Metadata.End = ad.adm.timeResolver.ResolveMonotonicTimestamp(ad.LoadConfig.EndTimestampRaw)
}

// SetTimeout updates the activity dump timeout
func (ad *ActivityDump) SetTimeout(timeout time.Duration) {
	ad.LoadConfig.SetTimeout(timeout)

	// Update metadata
	ad.Metadata.End = ad.adm.timeResolver.ResolveMonotonicTimestamp(ad.LoadConfig.EndTimestampRaw)
}

// updateTracedPid traces a pid in kernel space
func (ad *ActivityDump) updateTracedPid(pid uint32) {
	// start by looking up any existing entry
	var cookie uint32
	_ = ad.adm.tracedPIDsMap.Lookup(pid, &cookie)
	if cookie != ad.LoadConfigCookie {
		_ = ad.adm.tracedPIDsMap.Put(pid, ad.LoadConfigCookie)
	}
}

// commMatches returns true if the ActivityDump comm matches the provided comm
func (ad *ActivityDump) commMatches(comm string) bool {
	return ad.Metadata.Comm == comm
}

// nameMatches returns true if the ActivityDump name matches the provided name
func (ad *ActivityDump) nameMatches(name string) bool {
	return ad.Metadata.Name == name
}

// containerIDMatches returns true if the ActivityDump container ID matches the provided container ID
func (ad *ActivityDump) containerIDMatches(containerID string) bool {
	return ad.Metadata.ContainerID == containerID
}

// Matches returns true if the provided list of tags and / or the provided comm match the current ActivityDump
func (ad *ActivityDump) Matches(entry *model.ProcessCacheEntry) bool {
	if entry == nil {
		return false
	}

	if len(ad.Metadata.ContainerID) > 0 {
		if !ad.containerIDMatches(entry.ContainerID) {
			return false
		}
	}

	if len(ad.Metadata.Comm) > 0 {
		if !ad.commMatches(entry.Comm) {
			return false
		}
	}

	return true
}

// enable (thread unsafe) assuming the current dump is properly initialized, "enable" pushes kernel space filters so that events can start
// flowing in from kernel space
func (ad *ActivityDump) enable() error {
	// insert load config now (it might already exist, do not update in that case)
	if err := ad.adm.activityDumpsConfigMap.Put(ad.LoadConfigCookie, ad.LoadConfig); err != nil {
		return fmt.Errorf("couldn't push activity dump load config: %w", err)
	}

	if len(ad.Metadata.Comm) > 0 {
		commB := make([]byte, 16)
		copy(commB, ad.Metadata.Comm)
		err := ad.adm.tracedCommsMap.Put(commB, ad.LoadConfigCookie)
		if err != nil {
			return fmt.Errorf("couldn't push activity dump comm %s: %v", ad.Metadata.Comm, err)
		}
	}
	return nil
}

// pause (thread unsafe) assuming the current dump is running, "pause" sets the kernel space filters of the dump so that
// events are ignored in kernel space, and not sent to user space.
func (ad *ActivityDump) pause() error {
	if ad.state <= Paused {
		// nothing to do
		return nil
	}
	ad.state = Paused

	ad.LoadConfig.Paused = 1
	if err := ad.adm.activityDumpsConfigMap.Put(ad.LoadConfigCookie, ad.LoadConfig); err != nil {
		return fmt.Errorf("failed to pause activity dump [%s]: %w", ad.getSelectorStr(), err)
	}

	return nil
}

// removeLoadConfig (thread unsafe) removes the load config of a dump
func (ad *ActivityDump) removeLoadConfig() error {
	if err := ad.adm.activityDumpsConfigMap.Delete(ad.LoadConfigCookie); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("couldn't delete activity dump load config for dump [%s]: %w", ad.getSelectorStr(), err)
	}
	return nil
}

// disable (thread unsafe) assuming the current dump is running, "disable" removes kernel space filters so that events are no longer sent
// from kernel space
func (ad *ActivityDump) disable() error {
	if ad.state <= Disabled {
		// nothing to do
		return nil
	}
	ad.state = Disabled

	// remove activity dump config
	if err := ad.removeLoadConfig(); err != nil {
		return err
	}

	// remove comm from kernel space
	if len(ad.Metadata.Comm) > 0 {
		commB := make([]byte, 16)
		copy(commB, ad.Metadata.Comm)
		err := ad.adm.tracedCommsMap.Delete(commB)
		if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("couldn't delete activity dump filter comm(%s): %v", ad.Metadata.Comm, err)
		}
	}

	// remove container ID from kernel space
	if len(ad.Metadata.ContainerID) > 0 {
		containerIDB := make([]byte, model.ContainerIDLen)
		copy(containerIDB, ad.Metadata.ContainerID)
		err := ad.adm.tracedCgroupsMap.Delete(containerIDB)
		if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("couldn't delete activity dump filter containerID(%s): %v", ad.Metadata.ContainerID, err)
		}
	}
	return nil
}

// Finalize finalizes an active dump: envs and args are scrubbed, tags, service and container ID are set. If a cgroup
// spot can be released, the dump will be fully stopped.
func (ad *ActivityDump) Finalize(releaseTracedCgroupSpot bool) {
	ad.Lock()
	defer ad.Unlock()
	ad.finalize(releaseTracedCgroupSpot)
}

// finalize (thread unsafe) finalizes an active dump: envs and args are scrubbed, tags, service and container ID are set. If a cgroup
// spot can be released, the dump will be fully stopped.
func (ad *ActivityDump) finalize(releaseTracedCgroupSpot bool) {
	ad.Metadata.End = time.Now()

	if releaseTracedCgroupSpot || len(ad.Metadata.Comm) > 0 {
		if err := ad.disable(); err != nil {
			seclog.Errorf("couldn't disable activity dump: %v", err)
		}

		ad.state = Stopped
	}

	// add additional tags
	ad.adm.AddContextTags(ad)

	// look for the service tag and set the service of the dump
	ad.Service = utils.GetTagValue("service", ad.Tags)

	// add the container ID in a tag
	if len(ad.ContainerID) > 0 {
		ad.Tags = append(ad.Tags, "container_id:"+ad.ContainerID)
	}

	// scrub processes and retain args envs now
	ad.scrubAndRetainProcessArgsEnvs()
}

func (ad *ActivityDump) scrubAndRetainProcessArgsEnvs() {
	// iterate through all the process nodes
	openList := make([]*ProcessActivityNode, len(ad.ProcessActivityTree))
	copy(openList, ad.ProcessActivityTree)

	for len(openList) != 0 {
		current := openList[len(openList)-1]
		current.scrubAndReleaseArgsEnvs(ad.adm.processResolver)
		openList = append(openList[:len(openList)-1], current.Children...)
	}
}

// nolint: unused
func (ad *ActivityDump) debug(w io.Writer) {
	for _, root := range ad.ProcessActivityTree {
		root.debug(w, "")
	}
}

func (ad *ActivityDump) isEventTypeTraced(event *model.Event) bool {
	for _, evtType := range ad.LoadConfig.TracedEventTypes {
		if evtType == event.GetEventType() {
			return true
		}
	}
	return false
}

// IsEmpty return true if the dump did not contain any nodes
func (ad *ActivityDump) IsEmpty() bool {
	ad.Lock()
	defer ad.Unlock()
	return len(ad.ProcessActivityTree) == 0
}

// Insert inserts the provided event in the active ActivityDump. This function returns true if a new entry was added,
// false if the event was dropped.
func (ad *ActivityDump) Insert(event *model.Event) (newEntry bool) {
	ad.Lock()
	defer ad.Unlock()

	if ad.state != Running {
		// this activity dump is not running, ignore event
		return false
	}

	// check if this event type is traced
	if !ad.isEventTypeTraced(event) {
		// should not happen
		ad.eventTypeDrop[event.GetEventType()].Inc()
		return false
	}

	// metrics
	defer func() {
		if newEntry {
			// this doesn't count the exec events which are counted separately
			ad.addedRuntimeCount[event.GetEventType()].Inc()

			// check dump size
			ad.checkInMemorySize()
		}
	}()

	// find the node where the event should be inserted
	entry, _ := event.FieldHandlers.ResolveProcessCacheEntry(event)
	if entry == nil {
		return false
	}
	if !entry.HasCompleteLineage() { // check that the process context lineage is complete, otherwise drop it
		ad.brokenLineageDrop.Inc()
		return false
	}
	node := ad.findOrCreateProcessActivityNode(entry, Runtime)
	if node == nil {
		// a process node couldn't be found for the provided event as it doesn't match the ActivityDump query
		return false
	}

	// resolve fields
	event.ResolveFieldsForAD()

	// the count of processed events is the count of events that matched the activity dump selector = the events for
	// which we successfully found a process activity node
	ad.processedCount[event.GetEventType()].Inc()

	// insert the event based on its type
	switch event.GetEventType() {
	case model.FileOpenEventType:
		return ad.InsertFileEventInProcess(node, &event.Open.File, event, Runtime)
	case model.DNSEventType:
		return ad.InsertDNSEvent(node, &event.DNS, event.Rules)
	case model.BindEventType:
		return ad.InsertBindEvent(node, &event.Bind, event.Rules)
	case model.SyscallsEventType:
		// TODO (jrs): reactivate this tagging once we'll be able to write rules on used syscalls
		// for syscalls we tag the process node with the matched rule if any
		// node.MatchedRules = model.AppendMatchedRule(node.MatchedRules, event.Rules)
		return node.InsertSyscalls(&event.Syscalls)
	}

	// for process activity, tag the matched rule if any
	node.MatchedRules = model.AppendMatchedRule(node.MatchedRules, event.Rules)
	return false
}

// findOrCreateProcessActivityNode finds or a create a new process activity node in the activity dump if the entry
// matches the activity dump selector.
func (ad *ActivityDump) findOrCreateProcessActivityNode(entry *model.ProcessCacheEntry, generationType NodeGenerationType) (node *ProcessActivityNode) {
	if entry == nil {
		return node
	}

	// drop processes with abnormal paths
	if entry.GetPathResolutionError() != "" {
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

	defer func() {
		// if a node was found, and if the entry has a valid cookie, insert a cookie shortcut
		if entry.Cookie > 0 && node != nil {
			ad.CookiesNode[entry.Cookie] = node
		}
	}()

	// find or create a ProcessActivityNode for the parent of the input ProcessCacheEntry. If the parent is a fork entry,
	// jump immediately to the next ancestor.
	parentNode := ad.findOrCreateProcessActivityNode(entry.GetNextAncestorBinary(), Snapshot)

	// if parentNode is nil, the parent of the current node is out of tree (either because the parent is null, or it
	// doesn't match the dump tags).
	if parentNode == nil {

		// since the parent of the current entry wasn't inserted, we need to know if the current entry needs to be inserted.
		if !ad.Matches(entry) {
			return node
		}

		// go through the root nodes and check if one of them matches the input ProcessCacheEntry:
		for _, root := range ad.ProcessActivityTree {
			if root.Matches(&entry.Process, ad.Metadata.DifferentiateArgs) {
				return root
			}
		}

		// we're about to add a root process node, make sure this root node passes the root node sanitizer
		if !IsValidRootNode(&entry.ProcessContext) {
			ad.validRootNodeDrop.Inc()
			return node
		}

		// if it doesn't, create a new ProcessActivityNode for the input ProcessCacheEntry
		node = NewProcessActivityNode(entry, generationType, &ad.nodeStats)
		// insert in the list of root entries
		ad.ProcessActivityTree = append(ad.ProcessActivityTree, node)

	} else {

		// if parentNode wasn't nil, then (at least) the parent is part of the activity dump. This means that we need
		// to add the current entry no matter if it matches the selector or not. Go through the root children of the
		// parent node and check if one of them matches the input ProcessCacheEntry.
		for _, child := range parentNode.Children {
			if child.Matches(&entry.Process, ad.Metadata.DifferentiateArgs) {
				return child
			}
		}

		// if none of them matched, create a new ProcessActivityNode for the input processCacheEntry
		node = NewProcessActivityNode(entry, generationType, &ad.nodeStats)
		// insert in the list of children
		parentNode.Children = append(parentNode.Children, node)
	}

	// count new entry
	switch generationType {
	case Runtime:
		ad.addedRuntimeCount[model.ExecEventType].Inc()
	case Snapshot:
		ad.addedSnapshotCount[model.ExecEventType].Inc()
	}

	// set the pid of the input ProcessCacheEntry as traced
	ad.updateTracedPid(entry.Pid)

	// check dump size
	ad.checkInMemorySize()

	return node
}

// FindMatchingNodes return the matching nodes of requested comm
func (ad *ActivityDump) FindMatchingNodes(basename string) []*ProcessActivityNode {
	ad.Lock()
	defer ad.Unlock()

	var res []*ProcessActivityNode
	for _, node := range ad.ProcessActivityTree {
		if node.Process.FileEvent.BasenameStr == basename {
			res = append(res, node)
		}
	}

	return res
}

// GetImageNameTag returns the image name and tag for the profiled container
func (ad *ActivityDump) GetImageNameTag() (string, string) {
	ad.Lock()
	defer ad.Unlock()

	var imageName, imageTag string
	for _, tag := range ad.Tags {
		if tag_name, tag_value, valid := strings.Cut(tag, ":"); valid {
			switch tag_name {
			case "image_name":
				imageName = tag_value
			case "image_tag":
				imageTag = tag_value
			}
		}
	}
	return imageName, imageTag
}

// GetSelectorStr returns a string representation of the profile selector
func (ad *ActivityDump) GetSelectorStr() string {
	ad.Lock()
	defer ad.Unlock()

	return ad.getSelectorStr()
}

// getSelectorStr internal, thread-unsafe version of GetSelectorStr
func (ad *ActivityDump) getSelectorStr() string {
	tags := make([]string, 0, len(ad.Tags)+2)
	if len(ad.Metadata.ContainerID) > 0 {
		tags = append(tags, fmt.Sprintf("container_id:%s", ad.Metadata.ContainerID))
	}
	if len(ad.Metadata.Comm) > 0 {
		tags = append(tags, fmt.Sprintf("comm:%s", ad.Metadata.Comm))
	}
	if len(ad.Tags) > 0 {
		for _, tag := range ad.Tags {
			if !strings.HasPrefix(tag, "container_id") {
				tags = append(tags, tag)
			}
		}
	}
	if len(tags) == 0 {
		return "empty_selector"
	}
	return strings.Join(tags, ",")
}

// SendStats sends activity dump stats
func (ad *ActivityDump) SendStats() error {
	ad.Lock()
	defer ad.Unlock()

	for evtType, count := range ad.processedCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType)}
		if value := count.Swap(0); value > 0 {
			if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpEventProcessed, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventProcessed, err)
			}
		}
	}

	for evtType, count := range ad.addedRuntimeCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("generation_type:%s", Runtime)}
		if value := count.Swap(0); value > 0 {
			if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpEventAdded, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventAdded, err)
			}
		}
	}

	for evtType, count := range ad.addedSnapshotCount {
		tags := []string{fmt.Sprintf("event_type:%s", evtType), fmt.Sprintf("generation_type:%s", Snapshot)}
		if value := count.Swap(0); value > 0 {
			if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpEventAdded, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventAdded, err)
			}
		}
	}

	if value := ad.brokenLineageDrop.Swap(0); value > 0 {
		if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpBrokenLineageDrop, int64(value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpBrokenLineageDrop, err)
		}
	}

	for evtType, count := range ad.eventTypeDrop {
		tags := []string{fmt.Sprintf("event_type:%s", evtType)}
		if value := count.Swap(0); value > 0 {
			if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpEventTypeDrop, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEventTypeDrop, err)
			}
		}
	}

	if value := ad.validRootNodeDrop.Swap(0); value > 0 {
		if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpValidRootNodeDrop, int64(value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpValidRootNodeDrop, err)
		}
	}

	if value := ad.bindFamilyDrop.Swap(0); value > 0 {
		if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpBindFamilyDrop, int64(value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpBindFamilyDrop, err)
		}
	}

	if value := ad.pathMergedCount.Swap(0); value > 0 {
		if err := ad.adm.statsdClient.Count(metrics.MetricActivityDumpPathMergeCount, int64(value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpPathMergeCount, err)
		}
	}

	return nil
}

// Snapshot snapshots the processes in the activity dump to capture all the
func (ad *ActivityDump) Snapshot() error {
	ad.Lock()
	defer ad.Unlock()

	for _, pan := range ad.ProcessActivityTree {
		if err := ad.snapshotProcess(pan); err != nil {
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
	if len(ad.Tags) >= 10 || len(ad.Metadata.ContainerID) == 0 {
		return nil
	}

	var err error
	ad.Tags, err = ad.adm.tagsResolvers.ResolveWithErr(ad.Metadata.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", ad.Metadata.ContainerID, err)
	}

	return nil
}

// ToSecurityActivityDumpMessage returns a pointer to a SecurityActivityDumpMessage
func (ad *ActivityDump) ToSecurityActivityDumpMessage() *api.ActivityDumpMessage {
	ad.Lock()
	defer ad.Unlock()
	var storage []*api.StorageRequestMessage
	for _, requests := range ad.StorageRequests {
		for _, request := range requests {
			storage = append(storage, request.ToStorageRequestMessage(ad.Metadata.Name))
		}
	}

	return &api.ActivityDumpMessage{
		Host:    ad.Host,
		Source:  ad.Source,
		Service: ad.Service,
		Tags:    ad.Tags,
		Storage: storage,
		Metadata: &api.ActivityDumpMetadataMessage{
			AgentVersion:      ad.Metadata.AgentVersion,
			AgentCommit:       ad.Metadata.AgentCommit,
			KernelVersion:     ad.Metadata.KernelVersion,
			LinuxDistribution: ad.Metadata.LinuxDistribution,
			Name:              ad.Metadata.Name,
			ProtobufVersion:   ad.Metadata.ProtobufVersion,
			DifferentiateArgs: ad.Metadata.DifferentiateArgs,
			Comm:              ad.Metadata.Comm,
			ContainerID:       ad.Metadata.ContainerID,
			Start:             ad.Metadata.Start.Format(time.RFC822),
			Timeout:           ad.LoadConfig.Timeout.String(),
			Size:              ad.Metadata.Size,
			Arch:              ad.Metadata.Arch,
		},
		DNSNames: ad.DNSNames.Keys(),
	}
}

// ToTranscodingRequestMessage returns a pointer to a TranscodingRequestMessage
func (ad *ActivityDump) ToTranscodingRequestMessage() *api.TranscodingRequestMessage {
	var storage []*api.StorageRequestMessage
	for _, requests := range ad.StorageRequests {
		for _, request := range requests {
			storage = append(storage, request.ToStorageRequestMessage(ad.Metadata.Name))
		}
	}

	return &api.TranscodingRequestMessage{
		Storage: storage,
	}
}

// Encode encodes an activity dump in the provided format
func (ad *ActivityDump) Encode(format config.StorageFormat) (*bytes.Buffer, error) {
	switch format {
	case config.Json:
		return ad.EncodeJSON()
	case config.Protobuf:
		return ad.EncodeProtobuf()
	case config.Dot:
		return ad.EncodeDOT()
	case config.SecL:
		return ad.EncodeSecL()
	case config.Profile:
		return ad.EncodeProfile()
	default:
		return nil, fmt.Errorf("couldn't encode activity dump [%s] as [%s]: unknown format", ad.GetSelectorStr(), format)
	}
}

// EncodeProtobuf encodes an activity dump in the Protobuf format
func (ad *ActivityDump) EncodeProtobuf() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	pad := activityDumpToProto(ad)
	defer pad.ReturnToVTPool()

	raw, err := pad.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("couldn't encode in %s: %v", config.Protobuf, err)
	}
	return bytes.NewBuffer(raw), nil
}

// EncodeProfile encodes an activity dump in the Security Profile protobuf format
func (ad *ActivityDump) EncodeProfile() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	profileProto := ActivityDumpToSecurityProfileProto(ad)
	raw, err := profileProto.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("couldn't encode dump to `%s` format: %v", config.Profile, err)
	}
	return bytes.NewBuffer(raw), nil
}

// EncodeJSON encodes an activity dump in the ProtoJSON format
func (ad *ActivityDump) EncodeJSON() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	pad := activityDumpToProto(ad)
	defer pad.ReturnToVTPool()

	opts := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}

	raw, err := opts.Marshal(pad)
	if err != nil {
		return nil, fmt.Errorf("couldn't encode in %s: %v", config.Json, err)
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
	defer f.Close()

	seclog.Infof("unzipping %s", inputFile)
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("couldn't create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	outputFile, err := os.Create(strings.TrimSuffix(inputFile, ext))
	if err != nil {
		return "", fmt.Errorf("couldn't create gzip output file: %w", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, gzipReader)
	if err != nil {
		return "", fmt.Errorf("couldn't unzip %s: %w", inputFile, err)
	}

	if err = outputFile.Close(); err != nil {
		return "", fmt.Errorf("could not close file [%s]: %w", outputFile.Name(), err)
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

	format, err := config.ParseStorageFormat(ext)
	if err != nil {
		return err
	}

	f, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("couldn't open activity dump file: %w", err)
	}
	defer f.Close()

	return ad.DecodeFromReader(f, format)
}

// DecodeFromReader decodes an activity dump from a reader with the provided format
func (ad *ActivityDump) DecodeFromReader(reader io.Reader, format config.StorageFormat) error {
	switch format {
	case config.Protobuf:
		return ad.DecodeProtobuf(reader)
	default:
		return fmt.Errorf("unsupported input format: %s", format)
	}
}

// DecodeProtobuf decodes an activity dump as Protobuf
func (ad *ActivityDump) DecodeProtobuf(reader io.Reader) error {
	ad.Lock()
	defer ad.Unlock()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("couldn't open activity dump file: %w", err)
	}

	inter := &adproto.SecDump{}
	if err := inter.UnmarshalVT(raw); err != nil {
		return fmt.Errorf("couldn't decode protobuf activity dump file: %w", err)
	}

	protoToActivityDump(ad, inter)

	return nil
}

// ProcessActivityNode holds the activity of a process
type ProcessActivityNode struct {
	Process        model.Process
	GenerationType NodeGenerationType
	MatchedRules   []*model.MatchedRule

	Files    map[string]*FileActivityNode
	DNSNames map[string]*DNSNode
	Sockets  []*SocketNode
	Syscalls []int
	Children []*ProcessActivityNode
}

func (pan *ProcessActivityNode) getNodeLabel(args string) string {
	label := fmt.Sprintf("%s %s", pan.Process.FileEvent.PathnameStr, args)
	if len(pan.Process.FileEvent.PkgName) != 0 {
		label += fmt.Sprintf(" \\{%s %s\\}", pan.Process.FileEvent.PkgName, pan.Process.FileEvent.PkgVersion)
	}
	return label
}

// NewProcessActivityNode returns a new ProcessActivityNode instance
func NewProcessActivityNode(entry *model.ProcessCacheEntry, generationType NodeGenerationType, nodeStats *ActivityDumpNodeStats) *ProcessActivityNode {
	nodeStats.processNodes++
	return &ProcessActivityNode{
		Process:        entry.Process,
		GenerationType: generationType,
		Files:          make(map[string]*FileActivityNode),
		DNSNames:       make(map[string]*DNSNode),
	}
}

// nolint: unused
func (pan *ProcessActivityNode) debug(w io.Writer, prefix string) {
	fmt.Fprintf(w, "%s- process: %s\n", prefix, pan.Process.FileEvent.PathnameStr)
	if len(pan.Files) > 0 {
		fmt.Fprintf(w, "%s  files:\n", prefix)
		sortedFiles := make([]*FileActivityNode, 0, len(pan.Files))
		for _, f := range pan.Files {
			sortedFiles = append(sortedFiles, f)
		}
		sort.Slice(sortedFiles, func(i, j int) bool {
			return sortedFiles[i].Name < sortedFiles[j].Name
		})

		for _, f := range sortedFiles {
			f.debug(w, fmt.Sprintf("%s    -", prefix))
		}
	}
	if len(pan.Children) > 0 {
		fmt.Fprintf(w, "%s  children:\n", prefix)
		for _, child := range pan.Children {
			child.debug(w, prefix+"    ")
		}
	}
}

// scrubAndReleaseArgsEnvs scrubs the process args and envs, and then releases them
func (pan *ProcessActivityNode) scrubAndReleaseArgsEnvs(resolver *sprocess.Resolver) {
	if pan.Process.ArgsEntry != nil {
		_, _ = resolver.GetProcessScrubbedArgv(&pan.Process)
		pan.Process.Argv0, _ = resolver.GetProcessArgv0(&pan.Process)
		pan.Process.ArgsEntry = nil

	}
	if pan.Process.EnvsEntry != nil {
		envs, envsTruncated := resolver.GetProcessEnvs(&pan.Process)
		pan.Process.Envs = envs
		pan.Process.EnvsTruncated = envsTruncated
		pan.Process.EnvsEntry = nil
	}
}

// Matches return true if the process fields used to generate the dump are identical with the provided ProcessCacheEntry
func (pan *ProcessActivityNode) Matches(entry *model.Process, matchArgs bool) bool {
	if pan.Process.FileEvent.PathnameStr == entry.FileEvent.PathnameStr {
		if matchArgs {
			var panArgs, entryArgs []string
			if pan.Process.ArgsEntry != nil {
				panArgs, _ = sprocess.GetProcessArgv(&pan.Process)
			} else {
				panArgs = pan.Process.Argv
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

func ExtractFirstParent(path string) (string, int) {
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

// InsertFileEventInProcess inserts the provided file event in the current node. This function returns true if a new entry was
// added, false if the event was dropped.
func (ad *ActivityDump) InsertFileEventInProcess(pan *ProcessActivityNode, fileEvent *model.FileEvent, event *model.Event, generationType NodeGenerationType) bool {
	var filePath string
	if generationType != Snapshot {
		filePath = event.FieldHandlers.ResolveFilePath(event, fileEvent)
	} else {
		filePath = fileEvent.PathnameStr
	}

	// drop event for event with an error
	if event.Error != nil {
		return false
	}

	parent, nextParentIndex := ExtractFirstParent(filePath)
	if nextParentIndex == 0 {
		return false
	}

	// TODO: look for patterns / merge algo

	child, ok := pan.Files[parent]
	if ok {
		return ad.InsertFileEventInFile(child, fileEvent, event, fileEvent.PathnameStr[nextParentIndex:], generationType)
	}

	// create new child
	if len(fileEvent.PathnameStr) <= nextParentIndex+1 {
		node := NewFileActivityNode(fileEvent, event, parent, generationType, &ad.nodeStats)
		node.MatchedRules = model.AppendMatchedRule(node.MatchedRules, event.Rules)
		pan.Files[parent] = node
	} else {
		child := NewFileActivityNode(nil, nil, parent, generationType, &ad.nodeStats)
		ad.InsertFileEventInFile(child, fileEvent, event, fileEvent.PathnameStr[nextParentIndex:], generationType)
		pan.Files[parent] = child
	}
	return true
}

// snapshotProcess uses procfs to retrieve information about the current process
func (ad *ActivityDump) snapshotProcess(pan *ProcessActivityNode) error {
	// call snapshot for all the children of the current node
	for _, child := range pan.Children {
		if err := ad.snapshotProcess(child); err != nil {
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

	for _, eventType := range ad.LoadConfig.TracedEventTypes {
		switch eventType {
		case model.FileOpenEventType:
			if err = pan.snapshotFiles(p, ad); err != nil {
				return err
			}
		case model.BindEventType:
			if err = pan.snapshotBoundSockets(p, ad); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ad *ActivityDump) insertSnapshottedSocket(pan *ProcessActivityNode, p *process.Process, family uint16, ip net.IP, port uint16) {
	evt := ad.adm.newEvent()
	evt.Type = uint32(model.BindEventType)

	evt.Bind.SyscallEvent.Retval = 0
	evt.Bind.AddrFamily = family
	evt.Bind.Addr.IPNet.IP = ip
	if family == unix.AF_INET {
		evt.Bind.Addr.IPNet.Mask = net.CIDRMask(32, 32)
	} else {
		evt.Bind.Addr.IPNet.Mask = net.CIDRMask(128, 128)
	}
	evt.Bind.Addr.Port = port

	if ad.InsertBindEvent(pan, &evt.Bind, []*model.MatchedRule{}) {
		// count this new entry
		ad.addedSnapshotCount[model.BindEventType].Inc()
	}
}

func (pan *ProcessActivityNode) snapshotBoundSockets(p *process.Process, ad *ActivityDump) error {
	// list all the file descriptors opened by the process
	FDs, err := p.OpenFiles()
	if err != nil {
		return err
	}

	// sockets have the following pattern "socket:[inode]"
	var sockets []uint64
	for _, fd := range FDs {
		if strings.HasPrefix(fd.Path, "socket:[") {
			sock, err := strconv.Atoi(strings.TrimPrefix(fd.Path[:len(fd.Path)-1], "socket:["))
			if err != nil {
				return err
			}
			if sock < 0 {
				continue
			}
			sockets = append(sockets, uint64(sock))
		}
	}
	if len(sockets) <= 0 {
		return nil
	}

	// use /proc/[pid]/net/tcp,tcp6,udp,udp6 to extract the ports opened by the current process
	proc, _ := procfs.NewFS(filepath.Join(util.HostProc(fmt.Sprintf("%d", p.Pid))))
	if err != nil {
		return err
	}
	// looking for AF_INET sockets
	TCP, err := proc.NetTCP()
	if err != nil {
		seclog.Debugf("couldn't snapshot TCP sockets for [%s]: %v", ad.getSelectorStr(), err)
	}
	UDP, err := proc.NetUDP()
	if err != nil {
		seclog.Debugf("couldn't snapshot UDP sockets for [%s]: %v", ad.getSelectorStr(), err)
	}
	// looking for AF_INET6 sockets
	TCP6, err := proc.NetTCP6()
	if err != nil {
		seclog.Debugf("couldn't snapshot TCP6 sockets for [%s]: %v", ad.getSelectorStr(), err)
	}
	UDP6, err := proc.NetUDP6()
	if err != nil {
		seclog.Debugf("couldn't snapshot UDP6 sockets for [%s]: %v", ad.getSelectorStr(), err)
	}

	// searching for socket inode
	for _, s := range sockets {
		for _, sock := range TCP {
			if sock.Inode == s {
				ad.insertSnapshottedSocket(pan, p, unix.AF_INET, sock.LocalAddr, uint16(sock.LocalPort))
				break
			}
		}
		for _, sock := range UDP {
			if sock.Inode == s {
				ad.insertSnapshottedSocket(pan, p, unix.AF_INET, sock.LocalAddr, uint16(sock.LocalPort))
				break
			}
		}
		for _, sock := range TCP6 {
			if sock.Inode == s {
				ad.insertSnapshottedSocket(pan, p, unix.AF_INET6, sock.LocalAddr, uint16(sock.LocalPort))
				break
			}
		}
		for _, sock := range UDP6 {
			if sock.Inode == s {
				ad.insertSnapshottedSocket(pan, p, unix.AF_INET6, sock.LocalAddr, uint16(sock.LocalPort))
				break
			}
		}
		// not necessary found here, can be also another kind of socket (AF_UNIX, AF_NETLINK, etc)
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

		evt := ad.adm.newEvent()
		evt.Type = uint32(model.FileOpenEventType)

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
		evt.Open.File.FileFields.MTime = uint64(ad.adm.timeResolver.ComputeMonotonicTimestamp(time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec)))
		evt.Open.File.FileFields.CTime = uint64(ad.adm.timeResolver.ComputeMonotonicTimestamp(time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)))

		evt.Open.File.Mode = evt.Open.File.FileFields.Mode
		// TODO: add open flags by parsing `/proc/[pid]/fdinfo/fd` + O_RDONLY|O_CLOEXEC for the shared libs

		if ad.InsertFileEventInProcess(pan, &evt.Open.File, evt, Snapshot) {
			// count this new entry
			ad.addedSnapshotCount[model.FileOpenEventType].Inc()
		}
	}
	return nil
}

// InsertDNSEvent inserts
func (ad *ActivityDump) InsertDNSEvent(pan *ProcessActivityNode, evt *model.DNSEvent, rules []*model.MatchedRule) bool {
	ad.insertDNSNameToHackyBackendList(evt.Name)

	if dnsNode, ok := pan.DNSNames[evt.Name]; ok {
		dnsNode.MatchedRules = model.AppendMatchedRule(dnsNode.MatchedRules, rules)
		// look for the DNS request type
		for _, req := range dnsNode.Requests {
			if req.Type == evt.Type {
				return false
			}
		}

		// insert the new request
		dnsNode.Requests = append(dnsNode.Requests, *evt)
		return true
	}
	pan.DNSNames[evt.Name] = NewDNSNode(evt, &ad.nodeStats, rules)
	return true
}

func (ad *ActivityDump) insertDNSNameToHackyBackendList(name string) {
	if name == "" {
		return
	}

	ad.DNSNames.Insert(name)
}

// InsertBindEvent inserts a bind event to the activity dump
func (ad *ActivityDump) InsertBindEvent(pan *ProcessActivityNode, evt *model.BindEvent, rules []*model.MatchedRule) bool {
	if evt.SyscallEvent.Retval != 0 {
		return false
	}
	var newNode bool
	evtFamily := model.AddressFamily(evt.AddrFamily).String()

	// check if a socket of this type already exists
	var sock *SocketNode
	for _, s := range pan.Sockets {
		if s.Family == evtFamily {
			sock = s
		}
	}
	if sock == nil {
		sock = NewSocketNode(evtFamily, &ad.nodeStats)
		pan.Sockets = append(pan.Sockets, sock)
		newNode = true
	}

	// Insert bind event
	if sock.InsertBindEvent(ad, evt, rules) {
		newNode = true
	}

	return newNode
}

// IsValidRootNode evaluates if the provided process entry is allowed to become a root node of an Activity Dump
func IsValidRootNode(entry *model.ProcessContext) bool {
	// TODO: evaluate if the same issue affects other container runtimes
	return !(strings.HasPrefix(entry.FileEvent.BasenameStr, "runc") || strings.HasPrefix(entry.FileEvent.BasenameStr, "containerd-shim"))
}

// InsertSyscalls inserts the syscall of the process in the dump
func (pan *ProcessActivityNode) InsertSyscalls(e *model.SyscallsEvent) bool {
	var hasNewSyscalls bool
newSyscallLoop:
	for _, newSyscall := range e.Syscalls {
		for _, existingSyscall := range pan.Syscalls {
			if existingSyscall == int(newSyscall) {
				continue newSyscallLoop
			}
		}

		pan.Syscalls = append(pan.Syscalls, int(newSyscall))
		hasNewSyscalls = true
	}
	return hasNewSyscalls
}

// FileActivityNode holds a tree representation of a list of files
type FileActivityNode struct {
	MatchedRules   []*model.MatchedRule
	Name           string
	IsPattern      bool
	File           *model.FileEvent
	GenerationType NodeGenerationType
	FirstSeen      time.Time

	Open *OpenNode

	Children map[string]*FileActivityNode
}

// OpenNode contains the relevant fields of an Open event on which we might want to write a profiling rule
type OpenNode struct {
	model.SyscallEvent
	Flags uint32
	Mode  uint32
}

// NewFileActivityNode returns a new FileActivityNode instance
func NewFileActivityNode(fileEvent *model.FileEvent, event *model.Event, name string, generationType NodeGenerationType, nodeStats *ActivityDumpNodeStats) *FileActivityNode {
	nodeStats.fileNodes++
	fan := &FileActivityNode{
		Name:           name,
		GenerationType: generationType,
		Children:       make(map[string]*FileActivityNode),
	}
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
	if fan.File != nil {
		if len(fan.File.PkgName) != 0 {
			label += fmt.Sprintf(" \\{%s %s\\}", fan.File.PkgName, fan.File.PkgVersion)
		}
	}
	return label
}

func (fan *FileActivityNode) enrichFromEvent(event *model.Event) {
	if event == nil {
		return
	}
	if fan.FirstSeen.IsZero() {
		fan.FirstSeen = event.FieldHandlers.ResolveEventTimestamp(event)
	}

	fan.MatchedRules = model.AppendMatchedRule(fan.MatchedRules, event.Rules)

	switch event.GetEventType() {
	case model.FileOpenEventType:
		if fan.Open == nil {
			fan.Open = &OpenNode{
				SyscallEvent: event.Open.SyscallEvent,
				Flags:        event.Open.Flags,
				Mode:         event.Open.Mode,
			}
		} else {
			fan.Open.Flags |= event.Open.Flags
			fan.Open.Mode |= event.Open.Mode
		}
	}
}

// InsertFileEventInFile inserts an event in a FileActivityNode. This function returns true if a new entry was added, false if
// the event was dropped.
func (ad *ActivityDump) InsertFileEventInFile(fan *FileActivityNode, fileEvent *model.FileEvent, event *model.Event, remainingPath string, generationType NodeGenerationType) bool {
	currentFan := fan
	currentPath := remainingPath
	somethingChanged := false

	for {
		parent, nextParentIndex := ExtractFirstParent(currentPath)
		if nextParentIndex == 0 {
			currentFan.enrichFromEvent(event)
			break
		}

		if ad.shouldMergePaths && len(currentFan.Children) >= 10 {
			currentFan.Children = ad.combineChildren(currentFan.Children)
		}

		child, ok := currentFan.Children[parent]
		if ok {
			currentFan = child
			currentPath = currentPath[nextParentIndex:]
			continue
		}

		// create new child
		somethingChanged = true
		if len(currentPath) <= nextParentIndex+1 {
			currentFan.Children[parent] = NewFileActivityNode(fileEvent, event, parent, generationType, &ad.nodeStats)
			break
		} else {
			child := NewFileActivityNode(nil, nil, parent, generationType, &ad.nodeStats)
			currentFan.Children[parent] = child

			currentFan = child
			currentPath = currentPath[nextParentIndex:]
			continue
		}
	}
	return somethingChanged
}

func (ad *ActivityDump) combineChildren(children map[string]*FileActivityNode) map[string]*FileActivityNode {
	if len(children) == 0 {
		return children
	}

	type inner struct {
		pair utils.StringPair
		fan  *FileActivityNode
	}

	inputs := make([]inner, 0, len(children))
	for k, v := range children {
		inputs = append(inputs, inner{
			pair: utils.NewStringPair(k),
			fan:  v,
		})
	}

	current := []inner{inputs[0]}

	for _, a := range inputs[1:] {
		next := make([]inner, 0, len(current))
		shouldAppend := true
		for _, b := range current {
			if !areCompatibleFans(a.fan, b.fan) {
				next = append(next, b)
				continue
			}

			sp, similar := utils.BuildGlob(a.pair, b.pair, 4)
			if similar {
				spGlob, _ := sp.ToGlob()
				merged, ok := mergeFans(spGlob, a.fan, b.fan)
				if !ok {
					next = append(next, b)
					continue
				}

				if ad.nodeStats.fileNodes > 0 { // should not happen, but just to be sure
					ad.nodeStats.fileNodes--
				}
				next = append(next, inner{
					pair: sp,
					fan:  merged,
				})
				shouldAppend = false
			}
		}

		if shouldAppend {
			next = append(next, a)
		}
		current = next
	}

	mergeCount := len(inputs) - len(current)
	ad.pathMergedCount.Add(uint64(mergeCount))

	res := make(map[string]*FileActivityNode)
	for _, n := range current {
		glob, isPattern := n.pair.ToGlob()
		n.fan.Name = glob
		n.fan.IsPattern = isPattern
		res[glob] = n.fan
	}

	return res
}

func areCompatibleFans(a *FileActivityNode, b *FileActivityNode) bool {
	return reflect.DeepEqual(a.Open, b.Open)
}

func mergeFans(name string, a *FileActivityNode, b *FileActivityNode) (*FileActivityNode, bool) {
	newChildren := make(map[string]*FileActivityNode)
	for k, v := range a.Children {
		newChildren[k] = v
	}
	for k, v := range b.Children {
		if _, present := newChildren[k]; present {
			return nil, false
		}
		newChildren[k] = v
	}

	return &FileActivityNode{
		Name:           name,
		File:           a.File,
		GenerationType: a.GenerationType,
		FirstSeen:      a.FirstSeen,
		Open:           a.Open, // if the 2 fans are compatible, a.Open should be equal to b.Open
		Children:       newChildren,
		MatchedRules:   model.AppendMatchedRule(a.MatchedRules, b.MatchedRules),
	}, true
}

// nolint: unused
func (fan *FileActivityNode) debug(w io.Writer, prefix string) {
	fmt.Fprintf(w, "%s %s\n", prefix, fan.Name)

	sortedChildren := make([]*FileActivityNode, 0, len(fan.Children))
	for _, f := range fan.Children {
		sortedChildren = append(sortedChildren, f)
	}
	sort.Slice(sortedChildren, func(i, j int) bool {
		return sortedChildren[i].Name < sortedChildren[j].Name
	})

	for _, child := range sortedChildren {
		child.debug(w, "    "+prefix)
	}
}

// DNSNode is used to store a DNS node
type DNSNode struct {
	MatchedRules []*model.MatchedRule

	Requests []model.DNSEvent
}

// NewDNSNode returns a new DNSNode instance
func NewDNSNode(event *model.DNSEvent, nodeStats *ActivityDumpNodeStats, rules []*model.MatchedRule) *DNSNode {
	nodeStats.dnsNodes++
	return &DNSNode{
		MatchedRules: rules,
		Requests:     []model.DNSEvent{*event},
	}
}

// BindNode is used to store a bind node
type BindNode struct {
	MatchedRules []*model.MatchedRule

	Port uint16
	IP   string
}

// SocketNode is used to store a Socket node and associated events
type SocketNode struct {
	Family string
	Bind   []*BindNode
}

// InsertBindEvent inserts a bind even inside a socket node
func (n *SocketNode) InsertBindEvent(ad *ActivityDump, evt *model.BindEvent, rules []*model.MatchedRule) bool {
	// ignore non IPv4 / IPv6 bind events for now
	if evt.AddrFamily != unix.AF_INET && evt.AddrFamily != unix.AF_INET6 {
		ad.bindFamilyDrop.Inc()
		return false
	}
	evtIP := evt.Addr.IPNet.IP.String()

	for _, n := range n.Bind {
		if evt.Addr.Port == n.Port && evtIP == n.IP {
			n.MatchedRules = model.AppendMatchedRule(n.MatchedRules, rules)
			return false
		}
	}

	// insert bind event now
	n.Bind = append(n.Bind, &BindNode{
		MatchedRules: rules,
		Port:         evt.Addr.Port,
		IP:           evtIP,
	})
	return true
}

// NewSocketNode returns a new SocketNode instance
func NewSocketNode(family string, nodeStats *ActivityDumpNodeStats) *SocketNode {
	nodeStats.socketNodes++
	return &SocketNode{
		Family: family,
	}
}

// NodeGenerationType is used to indicate if a node was generated by a runtime or snapshot event
// IMPORTANT: IT MUST STAY IN SYNC WITH `adproto.GenerationType`
type NodeGenerationType byte

const (
	// Unknown is a node that was added at an unknown time
	Unknown NodeGenerationType = 0
	// Runtime is a node that was added at runtime
	Runtime NodeGenerationType = 1
	// Snapshot is a node that was added during the snapshot
	Snapshot NodeGenerationType = 2
)

func (genType NodeGenerationType) String() string {
	switch genType {
	case Runtime:
		return "runtime"
	case Snapshot:
		return "snapshot"
	default:
		return "unknown"
	}
}
