// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run golang.org/x/tools/cmd/stringer -type=HashState -linecomment -output model_string.go

// Package model holds model related files
package model

import (
	"net"
	"reflect"
	"time"
	"unsafe"

	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Model describes the data model for the runtime security agent events
type Model struct {
	ExtraValidateFieldFnc func(field eval.Field, fieldValue eval.FieldValue) error
}

var processContextZero = ProcessCacheEntry{}
var eventZero = Event{BaseEvent: BaseEvent{ContainerContext: &ContainerContext{}}}
var containerContextZero ContainerContext

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{
		BaseEvent: BaseEvent{
			ContainerContext: &ContainerContext{},
		},
	}
}

// NewDefaultEventWithType returns a new Event for the given type
func (m *Model) NewDefaultEventWithType(kind EventType) eval.Event {
	return &Event{
		BaseEvent: BaseEvent{
			Type:             uint32(kind),
			FieldHandlers:    &DefaultFieldHandlers{},
			ContainerContext: &ContainerContext{},
		},
	}
}

// Releasable represents an object than can be released
type Releasable struct {
	onReleaseCallback func() `field:"-" json:"-"`
}

// CallReleaseCallback calls the on-release callback
func (r *Releasable) CallReleaseCallback() {
	if r.onReleaseCallback != nil {
		r.onReleaseCallback()
	}
}

// SetReleaseCallback sets a callback to be called when the cache entry is released
func (r *Releasable) SetReleaseCallback(callback func()) {
	previousCallback := r.onReleaseCallback
	r.onReleaseCallback = func() {
		callback()
		if previousCallback != nil {
			previousCallback()
		}
	}
}

// OnRelease triggers the callback
func (r *Releasable) OnRelease() {
	r.onReleaseCallback()
}

// ContainerContext holds the container context of an event
type ContainerContext struct {
	Releasable
	ID        string   `field:"id,handler:ResolveContainerID"`                              // SECLDoc[id] Definition:`ID of the container`
	CreatedAt uint64   `field:"created_at,handler:ResolveContainerCreatedAt"`               // SECLDoc[created_at] Definition:`Timestamp of the creation of the container``
	Tags      []string `field:"tags,handler:ResolveContainerTags,opts:skip_ad,weight:9999"` // SECLDoc[tags] Definition:`Tags of the container`
	Resolved  bool     `field:"-"`
}

// Status defines the possible status of a profile as a bitmask
type Status uint32

const (
	// AnomalyDetection will trigger alerts each time an event is not part of the profile
	AnomalyDetection Status = 1 << iota
	// AutoSuppression will suppress any signal to events present on the profile
	AutoSuppression
	// WorkloadHardening will kill the process that triggered anomaly detection
	WorkloadHardening
)

// IsEnabled returns true if enabled
func (s Status) IsEnabled(option Status) bool {
	return (s & option) != 0
}

func (s Status) String() string {
	var options []string
	if s.IsEnabled(AnomalyDetection) {
		options = append(options, "anomaly_detection")
	}
	if s.IsEnabled(AutoSuppression) {
		options = append(options, "auto_suppression")
	}
	if s.IsEnabled(WorkloadHardening) {
		options = append(options, "workload_hardening")
	}

	var res string
	for _, option := range options {
		if len(res) > 0 {
			res += ","
		}
		res += option
	}
	return res
}

// SecurityProfileContext holds the security context of the profile
type SecurityProfileContext struct {
	Name                       string      `field:"name"`                          // SECLDoc[name] Definition:`Name of the security profile`
	Status                     Status      `field:"status"`                        // SECLDoc[status] Definition:`Status of the security profile`
	Version                    string      `field:"version"`                       // SECLDoc[version] Definition:`Version of the security profile`
	Tags                       []string    `field:"tags"`                          // SECLDoc[tags] Definition:`Tags of the security profile`
	AnomalyDetectionEventTypes []EventType `field:"anomaly_detection_event_types"` // SECLDoc[anomaly_detection_event_types] Definition:`Event types enabled for anomaly detection`
}

// CanGenerateAnomaliesFor returns true if the current profile can generate anomalies for the provided event type
func (spc SecurityProfileContext) CanGenerateAnomaliesFor(evtType EventType) bool {
	return slices.Contains(spc.AnomalyDetectionEventTypes, evtType)
}

// IPPortContext is used to hold an IP and Port
type IPPortContext struct {
	IPNet net.IPNet `field:"ip"`   // SECLDoc[ip] Definition:`IP address`
	Port  uint16    `field:"port"` // SECLDoc[port] Definition:`Port number`
}

// NetworkContext represents the network context of the event
type NetworkContext struct {
	Device NetworkDeviceContext `field:"device"` // network device on which the network packet was captured

	L3Protocol  uint16        `field:"l3_protocol"` // SECLDoc[l3_protocol] Definition:`l3 protocol of the network packet` Constants:`L3 protocols`
	L4Protocol  uint16        `field:"l4_protocol"` // SECLDoc[l4_protocol] Definition:`l4 protocol of the network packet` Constants:`L4 protocols`
	Source      IPPortContext `field:"source"`      // source of the network packet
	Destination IPPortContext `field:"destination"` // destination of the network packet
	Size        uint32        `field:"size"`        // SECLDoc[size] Definition:`size in bytes of the network packet`
}

// SpanContext describes a span context
type SpanContext struct {
	SpanID  uint64 `field:"_" json:"-"`
	TraceID uint64 `field:"_" json:"-"`
}

// BaseEvent represents an event sent from the kernel
type BaseEvent struct {
	ID           string         `field:"-" event:"*"`
	Type         uint32         `field:"-"`
	Flags        uint32         `field:"-"`
	TimestampRaw uint64         `field:"event.timestamp,handler:ResolveEventTimestamp" event:"*"` // SECLDoc[event.timestamp] Definition:`Timestamp of the event`
	Timestamp    time.Time      `field:"-"`
	Rules        []*MatchedRule `field:"-"`

	// context shared with all events
	SpanContext            SpanContext            `field:"-" json:"-"`
	ProcessContext         *ProcessContext        `field:"process" event:"*"`
	ContainerContext       *ContainerContext      `field:"container" event:"*"`
	NetworkContext         NetworkContext         `field:"network" event:"dns"`
	SecurityProfileContext SecurityProfileContext `field:"-"`

	// internal usage
	PIDContext        PIDContext         `field:"-" json:"-"`
	ProcessCacheEntry *ProcessCacheEntry `field:"-" json:"-"`

	// mark event with having error
	Error error `field:"-" json:"-"`

	// field resolution
	FieldHandlers FieldHandlers `field:"-" json:"-"`
}

func initMember(member reflect.Value, deja map[string]bool) {
	for i := 0; i < member.NumField(); i++ {
		field := member.Field(i)

		switch field.Kind() {
		case reflect.Ptr:
			if field.CanSet() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			if field.Elem().Kind() == reflect.Struct {
				name := field.Elem().Type().Name()
				if deja[name] {
					continue
				}
				deja[name] = true

				initMember(field.Elem(), deja)
			}
		case reflect.Struct:
			name := field.Type().Name()
			if deja[name] {
				continue
			}
			deja[name] = true

			initMember(field, deja)
		}
	}
}

// NewDefaultEvent returns a new event using the default field handlers
func NewDefaultEvent() *Event {
	return &Event{
		BaseEvent: BaseEvent{
			FieldHandlers:    &DefaultFieldHandlers{},
			ContainerContext: &ContainerContext{},
		},
	}
}

// Init initialize the event
func (e *Event) Init() {
	initMember(reflect.ValueOf(e).Elem(), map[string]bool{})
}

// Zero the event
func (e *Event) Zero() {
	*e = eventZero
	*e.BaseEvent.ContainerContext = containerContextZero
}

// IsSavedByActivityDumps return whether saved by AD
func (e *Event) IsSavedByActivityDumps() bool {
	return e.Flags&EventFlagsSavedByAD > 0
}

// IsActivityDumpSample return whether AD sample
func (e *Event) IsActivityDumpSample() bool {
	return e.Flags&EventFlagsActivityDumpSample > 0
}

// IsInProfile return true if the event was found in the profile
func (e *Event) IsInProfile() bool {
	return e.Flags&EventFlagsSecurityProfileInProfile > 0
}

// IsKernelSpaceAnomalyDetectionEvent returns true if the event is a kernel space anomaly detection event
func (e *Event) IsKernelSpaceAnomalyDetectionEvent() bool {
	return AnomalyDetectionSyscallEventType == e.GetEventType()
}

// IsAnomalyDetectionEvent returns true if the current event is an anomaly detection event (kernel or user space)
func (e *Event) IsAnomalyDetectionEvent() bool {
	if !e.SecurityProfileContext.Status.IsEnabled(AnomalyDetection) {
		return false
	}

	// first, check if the current event is a kernel generated anomaly detection event
	if e.IsKernelSpaceAnomalyDetectionEvent() {
		return true
	} else if !e.SecurityProfileContext.CanGenerateAnomaliesFor(e.GetEventType()) {
		// the profile can't generate anomalies for the current event type
		return false
	} else if e.IsInProfile() {
		return false
	}
	return true
}

// AddToFlags adds a flag to the event
func (e *Event) AddToFlags(flag uint32) {
	e.Flags |= flag
}

// RemoveFromFlags remove a flag to the event
func (e *Event) RemoveFromFlags(flag uint32) {
	e.Flags ^= flag
}

// HasProfile returns true if we found a profile for that event
func (e *Event) HasProfile() bool {
	return e.SecurityProfileContext.Name != ""
}

// GetType returns the event type
func (e *Event) GetType() string {
	return EventType(e.Type).String()
}

// GetEventType returns the event type of the event
func (e *Event) GetEventType() EventType {
	return EventType(e.Type)
}

// GetTags returns the list of tags specific to this event
func (e *Event) GetTags() []string {
	tags := []string{"type:" + e.GetType()}

	// should already be resolved at this stage
	if len(e.ContainerContext.Tags) > 0 {
		tags = append(tags, e.ContainerContext.Tags...)
	}
	return tags
}

// GetWorkloadID returns an ID that represents the workload
func (e *Event) GetWorkloadID() string {
	return e.SecurityProfileContext.Name
}

// Retain the event
func (e *Event) Retain() Event {
	if e.ProcessCacheEntry != nil {
		e.ProcessCacheEntry.Retain()
	}
	return *e
}

// Release the event
func (e *Event) Release() {
	if e.ProcessCacheEntry != nil {
		e.ProcessCacheEntry.Release()
	}
}

// ResolveProcessCacheEntry uses the field handler
func (e *Event) ResolveProcessCacheEntry() (*ProcessCacheEntry, bool) {
	return e.FieldHandlers.ResolveProcessCacheEntry(e)
}

// ResolveEventTime uses the field handler
func (e *Event) ResolveEventTime() time.Time {
	return e.FieldHandlers.ResolveEventTime(e)
}

// GetProcessService uses the field handler
func (e *Event) GetProcessService() string {
	return e.FieldHandlers.GetProcessService(e)
}

// MatchedRule contains the identification of one rule that has match
type MatchedRule struct {
	RuleID        string
	RuleVersion   string
	RuleTags      map[string]string
	PolicyName    string
	PolicyVersion string
}

// NewMatchedRule return a new MatchedRule instance
func NewMatchedRule(ruleID, ruleVersion string, ruleTags map[string]string, policyName, policyVersion string) *MatchedRule {
	return &MatchedRule{
		RuleID:        ruleID,
		RuleVersion:   ruleVersion,
		RuleTags:      ruleTags,
		PolicyName:    policyName,
		PolicyVersion: policyVersion,
	}
}

// Match returns true if the rules are equal
func (mr *MatchedRule) Match(mr2 *MatchedRule) bool {
	if mr2 == nil ||
		mr.RuleID != mr2.RuleID ||
		mr.RuleVersion != mr2.RuleVersion ||
		mr.PolicyName != mr2.PolicyName ||
		mr.PolicyVersion != mr2.PolicyVersion {
		return false
	}
	return true
}

// AppendMatchedRule appends two lists, but avoiding duplicates
func AppendMatchedRule(list []*MatchedRule, toAdd []*MatchedRule) []*MatchedRule {
	for _, ta := range toAdd {
		found := false
		for _, l := range list {
			if l.Match(ta) { // rule already present
				found = true
				break
			}
		}
		if !found {
			list = append(list, ta)
		}
	}
	return list
}

// HashState is used to prevent the hash resolver from retrying to hash a file
type HashState int

const (
	// NoHash means that computing a hash hasn't been attempted
	NoHash HashState = iota
	// Done means that the hashes were already computed
	Done
	// FileNotFound means that the underlying file is not longer available to compute the hash
	FileNotFound
	// PathnameResolutionError means that the underlying file wasn't properly resolved
	PathnameResolutionError
	// FileTooBig means that the underlying file is larger than the hash resolver file size limit
	FileTooBig
	// FileEmpty means that the underlying file is empty
	FileEmpty
	// FileOpenError is a generic hash state to say that we couldn't open the file
	FileOpenError
	// EventTypeNotConfigured means that the event type prevents a hash from being computed
	EventTypeNotConfigured
	// HashWasRateLimited means that the hash will be tried again later, it was rate limited
	HashWasRateLimited
	// MaxHashState is used for initializations
	MaxHashState
)

// HashAlgorithm is used to configure the hash algorithms of the hash resolver
type HashAlgorithm int

const (
	// SHA1 is used to identify a SHA1 hash
	SHA1 HashAlgorithm = iota
	// SHA256 is used to identify a SHA256 hash
	SHA256
	// MD5 is used to identify a MD5 hash
	MD5
	// MaxHashAlgorithm is used for initializations
	MaxHashAlgorithm
)

func (ha HashAlgorithm) String() string {
	switch ha {
	case SHA1:
		return "sha1"
	case SHA256:
		return "sha256"
	case MD5:
		return "md5"
	default:
		return ""
	}
}

var zeroProcessContext ProcessContext

// ProcessCacheEntry this struct holds process context kept in the process tree
type ProcessCacheEntry struct {
	ProcessContext

	refCount  uint64                     `field:"-" json:"-"`
	onRelease func(_ *ProcessCacheEntry) `field:"-" json:"-"`
	releaseCb func()                     `field:"-" json:"-"`
}

// IsContainerRoot returns whether this is a top level process in the container ID
func (pc *ProcessCacheEntry) IsContainerRoot() bool {
	return pc.ContainerID != "" && pc.Ancestor != nil && pc.Ancestor.ContainerID == ""
}

// Reset the entry
func (pc *ProcessCacheEntry) Reset() {
	pc.ProcessContext = zeroProcessContext
	pc.refCount = 0
	pc.releaseCb = nil
}

// Retain increment ref counter
func (pc *ProcessCacheEntry) Retain() {
	pc.refCount++
}

// SetReleaseCallback set the callback called when the entry is released
func (pc *ProcessCacheEntry) SetReleaseCallback(callback func()) {
	previousCallback := pc.releaseCb
	pc.releaseCb = func() {
		callback()
		if previousCallback != nil {
			previousCallback()
		}
	}
}

// Release decrement and eventually release the entry
func (pc *ProcessCacheEntry) Release() {
	pc.refCount--
	if pc.refCount > 0 {
		return
	}

	if pc.onRelease != nil {
		pc.onRelease(pc)
	}

	if pc.releaseCb != nil {
		pc.releaseCb()
	}
}

// NewProcessCacheEntry returns a new process cache entry
func NewProcessCacheEntry(onRelease func(_ *ProcessCacheEntry)) *ProcessCacheEntry {
	return &ProcessCacheEntry{onRelease: onRelease}
}

// ProcessAncestorsIterator defines an iterator of ancestors
type ProcessAncestorsIterator struct {
	prev *ProcessCacheEntry
}

// Front returns the first element
func (it *ProcessAncestorsIterator) Front(ctx *eval.Context) unsafe.Pointer {
	if front := ctx.Event.(*Event).ProcessContext.Ancestor; front != nil {
		it.prev = front
		return unsafe.Pointer(front)
	}

	return nil
}

// Next returns the next element
func (it *ProcessAncestorsIterator) Next() unsafe.Pointer {
	if next := it.prev.Ancestor; next != nil {
		it.prev = next
		return unsafe.Pointer(next)
	}

	return nil
}

// HasParent returns whether the process has a parent
func (p *ProcessContext) HasParent() bool {
	return p.Parent != nil
}

// ProcessContext holds the process context of an event
type ProcessContext struct {
	Process

	Parent   *Process           `field:"parent,opts:exposed_at_event_root_only,check:HasParent"`
	Ancestor *ProcessCacheEntry `field:"ancestors,iterator:ProcessAncestorsIterator,check:IsNotKworker"`
}

// ExitEvent represents a process exit event
type ExitEvent struct {
	*Process
	Cause uint32 `field:"cause"` // SECLDoc[cause] Definition:`Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)`
	Code  uint32 `field:"code"`  // SECLDoc[code] Definition:`Exit code of the process or number of the signal that caused the process to terminate`
}

// DNSEvent represents a DNS event
type DNSEvent struct {
	ID    uint16 `field:"id" json:"-"`                                             // SECLDoc[id] Definition:`[Experimental] the DNS request ID`
	Name  string `field:"question.name,opts:length" op_override:"eval.DNSNameCmp"` // SECLDoc[question.name] Definition:`the queried domain name`
	Type  uint16 `field:"question.type"`                                           // SECLDoc[question.type] Definition:`a two octet code which specifies the DNS question type` Constants:`DNS qtypes`
	Class uint16 `field:"question.class"`                                          // SECLDoc[question.class] Definition:`the class looked up by the DNS question` Constants:`DNS qclasses`
	Size  uint16 `field:"question.length"`                                         // SECLDoc[question.length] Definition:`the total DNS request size in bytes`
	Count uint16 `field:"question.count"`                                          // SECLDoc[question.count] Definition:`the total count of questions in the DNS request`
}

// ExtraFieldHandlers handlers not hold by any field
type ExtraFieldHandlers interface {
	ResolveProcessCacheEntry(ev *Event) (*ProcessCacheEntry, bool)
	ResolveContainerContext(ev *Event) (*ContainerContext, bool)
	ResolveEventTime(ev *Event) time.Time
	GetProcessService(ev *Event) string
	ResolveHashes(eventType EventType, process *Process, file *FileEvent) []string
}

// ResolveProcessCacheEntry stub implementation
func (dfh *DefaultFieldHandlers) ResolveProcessCacheEntry(ev *Event) (*ProcessCacheEntry, bool) {
	return nil, false
}

// ResolveContainerContext stub implementation
func (dfh *DefaultFieldHandlers) ResolveContainerContext(ev *Event) (*ContainerContext, bool) {
	return nil, false
}

// ResolveEventTime stub implementation
func (dfh *DefaultFieldHandlers) ResolveEventTime(ev *Event) time.Time {
	return ev.Timestamp
}

// GetProcessService stub implementation
func (dfh *DefaultFieldHandlers) GetProcessService(ev *Event) string {
	return ""
}

// ResolveHashes resolves the hash of the provided file
func (dfh *DefaultFieldHandlers) ResolveHashes(eventType EventType, process *Process, file *FileEvent) []string {
	return nil
}
