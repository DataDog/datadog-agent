// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

// Package dump holds dump related files
package dump

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/encoding/protojson"

	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	stime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
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

// ActivityDump holds the activity tree for the workload defined by the provided list of tags. The encoding described by
// the `msg` annotation is used to generate the activity dump file while the encoding described by the `json` annotation
// is used to generate the activity dump metadata sent to the event platform.
// easyjson:json
type ActivityDump struct {
	*sync.Mutex
	state    ActivityDumpStatus
	adm      *ActivityDumpManager
	selector *cgroupModel.WorkloadSelector

	countedByLimiter bool

	// standard attributes used by the intake
	Host    string   `json:"host,omitempty"`
	Service string   `json:"service,omitempty"`
	Source  string   `json:"ddsource,omitempty"`
	Tags    []string `json:"-"`
	DDTags  string   `json:"ddtags,omitempty"`

	ActivityTree    *activity_tree.ActivityTree                      `json:"-"`
	StorageRequests map[config.StorageFormat][]config.StorageRequest `json:"-"`

	// Dump metadata
	mtdt.Metadata

	// Used to store the global list of DNS names contained in this dump
	// this is a hack used to provide this global list to the backend in the JSON header
	// instead of in the protobuf payload.
	DNSNames *utils.StringKeys `json:"dns_names"`

	// Load config
	LoadConfig       *model.ActivityDumpLoadConfig `json:"-"`
	LoadConfigCookie uint64                        `json:"-"`
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
func NewEmptyActivityDump(pathsReducer *activity_tree.PathsReducer) *ActivityDump {
	ad := &ActivityDump{
		Mutex:           &sync.Mutex{},
		StorageRequests: make(map[config.StorageFormat][]config.StorageRequest),
		DNSNames:        utils.NewStringKeys(nil),
	}
	ad.ActivityTree = activity_tree.NewActivityTree(ad, pathsReducer, "activity_dump")
	return ad
}

// WithDumpOption can be used to configure an ActivityDump
type WithDumpOption func(ad *ActivityDump)

// NewActivityDump returns a new instance of an ActivityDump
func NewActivityDump(adm *ActivityDumpManager, options ...WithDumpOption) *ActivityDump {
	ad := NewEmptyActivityDump(adm.pathsReducer)
	now := time.Now()
	ad.Metadata = mtdt.Metadata{
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

	// set load configuration to initial defaults
	ad.LoadConfig = NewActivityDumpLoadConfig(
		adm.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		adm.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout,
		adm.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		adm.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
		adm.resolvers.TimeResolver,
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

	ad := NewEmptyActivityDump(nil)
	ad.Host = msg.GetHost()
	ad.Service = msg.GetService()
	ad.Source = msg.GetSource()
	ad.Tags = msg.GetTags()
	ad.Metadata = mtdt.Metadata{
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

// GetWorkloadSelector returns the workload selector of the dump
func (ad *ActivityDump) GetWorkloadSelector() *cgroupModel.WorkloadSelector {
	if ad.selector != nil && ad.selector.IsReady() {
		return ad.selector
	}
	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", ad.Tags), utils.GetTagValue("image_tag", ad.Tags))
	if err != nil {
		return nil
	}
	ad.selector = &selector
	return ad.selector
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
	return ad.ActivityTree.Stats.ApproximateSize()
}

// SetLoadConfig set the load config of the current activity dump
func (ad *ActivityDump) SetLoadConfig(cookie uint64, config model.ActivityDumpLoadConfig) {
	ad.LoadConfig = &config
	ad.LoadConfigCookie = cookie

	// Update metadata
	ad.Metadata.Start = ad.adm.resolvers.TimeResolver.ResolveMonotonicTimestamp(ad.LoadConfig.StartTimestampRaw)
	ad.Metadata.End = ad.adm.resolvers.TimeResolver.ResolveMonotonicTimestamp(ad.LoadConfig.EndTimestampRaw)
}

// SetTimeout updates the activity dump timeout
func (ad *ActivityDump) SetTimeout(timeout time.Duration) {
	ad.LoadConfig.SetTimeout(timeout)

	// Update metadata
	ad.Metadata.End = ad.adm.resolvers.TimeResolver.ResolveMonotonicTimestamp(ad.LoadConfig.EndTimestampRaw)
}

// updateTracedPid traces a pid in kernel space
func (ad *ActivityDump) updateTracedPid(pid uint32) {
	// start by looking up any existing entry
	var cookie uint64
	if ad.adm != nil { // it could be nil when running unit tests
		_ = ad.adm.tracedPIDsMap.Lookup(pid, &cookie)
		if cookie != ad.LoadConfigCookie {
			_ = ad.adm.tracedPIDsMap.Put(pid, ad.LoadConfigCookie)
		}
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

// MatchesSelector returns true if the provided list of tags and / or the provided comm match the current ActivityDump
func (ad *ActivityDump) MatchesSelector(entry *model.ProcessCacheEntry) bool {
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

// IsEventTypeValid returns true if the provided event type is traced by the activity dump
func (ad *ActivityDump) IsEventTypeValid(event model.EventType) bool {
	return slices.Contains(ad.LoadConfig.TracedEventTypes, event)
}

// NewProcessNodeCallback is a callback function used to propagate the fact that a new process node was added to the
// activity tree
func (ad *ActivityDump) NewProcessNodeCallback(p *activity_tree.ProcessNode) {
	// set the pid of the input ProcessCacheEntry as traced
	ad.updateTracedPid(p.Process.Pid)
}

// enable (thread unsafe) assuming the current dump is properly initialized, "enable" pushes kernel space filters so that events can start
// flowing in from kernel space
func (ad *ActivityDump) enable() error {
	// insert load config now (it might already exist when starting a new partial dump, update it in that case)
	if err := ad.adm.activityDumpsConfigMap.Update(ad.LoadConfigCookie, ad.LoadConfig, ebpf.UpdateAny); err != nil {
		if !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("couldn't push activity dump load config: %w", err)
		}
	}

	if len(ad.Metadata.ContainerID) > 0 {
		// insert container ID in traced_cgroups map (it might already exist, do not update in that case)
		if err := ad.adm.tracedCgroupsMap.Update(ad.Metadata.ContainerID, ad.LoadConfigCookie, ebpf.UpdateNoExist); err != nil {
			if !errors.Is(err, ebpf.ErrKeyExist) {
				// delete activity dump load config
				_ = ad.adm.activityDumpsConfigMap.Delete(ad.LoadConfigCookie)
				return fmt.Errorf("couldn't push activity dump container ID %s: %w", ad.Metadata.ContainerID, err)
			}
		}
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
	ad.adm.lastStoppedDumpTime = ad.Metadata.End

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
	ad.ActivityTree.ScrubProcessArgsEnvs(ad.adm.resolvers.ProcessResolver)
}

// IsEmpty return true if the dump did not contain any nodes
func (ad *ActivityDump) IsEmpty() bool {
	ad.Lock()
	defer ad.Unlock()
	return ad.ActivityTree.IsEmpty()
}

// Insert inserts the provided event in the active ActivityDump. This function returns true if a new entry was added,
// false if the event was dropped.
func (ad *ActivityDump) Insert(event *model.Event) {
	ad.Lock()
	defer ad.Unlock()

	if ad.state != Running {
		// this activity dump is not running, ignore event
		return
	}

	if ok, err := ad.ActivityTree.Insert(event, activity_tree.Runtime, ad.adm.resolvers); ok && err == nil {
		// check dump size
		ad.checkInMemorySize()
	}

	return
}

// FindMatchingRootNodes return the matching nodes of requested comm
func (ad *ActivityDump) FindMatchingRootNodes(basename string) []*activity_tree.ProcessNode {
	ad.Lock()
	defer ad.Unlock()
	return ad.ActivityTree.FindMatchingRootNodes(basename)
}

// GetImageNameTag returns the image name and tag for the profiled container
func (ad *ActivityDump) GetImageNameTag() (string, string) {
	ad.Lock()
	defer ad.Unlock()

	var imageName, imageTag string
	for _, tag := range ad.Tags {
		if tagName, tagValue, valid := strings.Cut(tag, ":"); valid {
			switch tagName {
			case "image_name":
				imageName = tagValue
			case "image_tag":
				imageTag = tagValue
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
	return ad.ActivityTree.SendStats(ad.adm.statsdClient)
}

// Snapshot snapshots the processes in the activity dump to capture all the
func (ad *ActivityDump) Snapshot() error {
	ad.Lock()
	defer ad.Unlock()

	ad.ActivityTree.Snapshot(ad.adm.newEvent)

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
	ad.Tags, err = ad.adm.resolvers.TagsResolver.ResolveWithErr(ad.Metadata.ContainerID)
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

	msg := &api.ActivityDumpMessage{
		Host:    ad.Host,
		Source:  ad.Source,
		Service: ad.Service,
		Tags:    ad.Tags,
		Storage: storage,
		Metadata: &api.MetadataMessage{
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
	if ad.ActivityTree != nil {
		msg.Stats = &api.ActivityTreeStatsMessage{
			ProcessNodesCount: ad.ActivityTree.Stats.ProcessNodes,
			FileNodesCount:    ad.ActivityTree.Stats.FileNodes,
			DNSNodesCount:     ad.ActivityTree.Stats.DNSNodes,
			SocketNodesCount:  ad.ActivityTree.Stats.SocketNodes,
			ApproximateSize:   ad.ActivityTree.Stats.ApproximateSize(),
		}
	}
	return msg
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
	case config.JSON:
		return ad.EncodeJSON("")
	case config.Protobuf:
		return ad.EncodeProtobuf()
	case config.Dot:
		return ad.EncodeDOT()
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
func (ad *ActivityDump) EncodeJSON(indent string) (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	pad := activityDumpToProto(ad)
	defer pad.ReturnToVTPool()

	opts := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
		Indent:          indent,
	}

	raw, err := opts.Marshal(pad)
	if err != nil {
		return nil, fmt.Errorf("couldn't encode in %s: %v", config.JSON, err)
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
	case config.Profile:
		return ad.DecodeProfileProtobuf(reader)
	case config.JSON:
		return ad.DecodeJSON(reader)
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

	var pathsReducer *activity_tree.PathsReducer
	if ad.adm != nil {
		pathsReducer = ad.adm.pathsReducer
	}

	protoToActivityDump(ad, pathsReducer, inter)

	return nil
}

// DecodeProfileProtobuf decodes an activity dump from a profile protobuf
func (ad *ActivityDump) DecodeProfileProtobuf(reader io.Reader) error {
	ad.Lock()
	defer ad.Unlock()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("couldn't open security profile file: %w", err)
	}

	inter := &adproto.SecurityProfile{}
	if err := inter.UnmarshalVT(raw); err != nil {
		return fmt.Errorf("couldn't decode protobuf activity dump file: %w", err)
	}

	var reducer *activity_tree.PathsReducer
	if ad.adm != nil {
		reducer = ad.adm.pathsReducer
	}

	securityProfileProtoToActivityDump(ad, reducer, inter)

	return nil
}

// DecodeJSON decodes JSON to an activity dump
func (ad *ActivityDump) DecodeJSON(reader io.Reader) error {
	ad.Lock()
	defer ad.Unlock()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("couldn't open security profile file: %w", err)
	}

	opts := protojson.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
	}
	inter := &adproto.SecDump{}
	if err = opts.Unmarshal(raw, inter); err != nil {
		return fmt.Errorf("couldn't decode json file: %w", err)
	}

	var reducer *activity_tree.PathsReducer
	if ad.adm != nil {
		reducer = ad.adm.pathsReducer
	}

	protoToActivityDump(ad, reducer, inter)

	return nil
}
