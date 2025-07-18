// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate stringer -type=HashState -linecomment -output model_string.go

// Package model holds model related files
package model

import (
	"net"
	"net/netip"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// Model describes the data model for the runtime security agent events
type Model struct {
	ExtraValidateFieldFnc func(field eval.Field, fieldValue eval.FieldValue) error
}

var containerContextZero ContainerContext

// Releasable represents an object than can be released
type Releasable struct {
	onReleaseCallbacks []func() `field:"-"`
}

// CallReleaseCallback calls the on-release callback
func (r *Releasable) CallReleaseCallback() {
	for _, cb := range r.onReleaseCallbacks {
		cb()
	}
}

// AppendReleaseCallback sets a callback to be called when the cache entry is released
func (r *Releasable) AppendReleaseCallback(callback func()) {
	if callback != nil {
		r.onReleaseCallbacks = append(r.onReleaseCallbacks, callback)
	}
}

// ContainerContext holds the container context of an event
type ContainerContext struct {
	Releasable
	ContainerID containerutils.ContainerID `field:"id,handler:ResolveContainerID,opts:gen_getters"`                // SECLDoc[id] Definition:`ID of the container`
	CreatedAt   uint64                     `field:"created_at,handler:ResolveContainerCreatedAt,opts:gen_getters"` // SECLDoc[created_at] Definition:`Timestamp of the creation of the container``
	Tags        []string                   `field:"tags,handler:ResolveContainerTags,opts:skip_ad,weight:9999"`    // SECLDoc[tags] Definition:`Tags of the container`
	Resolved    bool                       `field:"-"`
	Runtime     string                     `field:"runtime,handler:ResolveContainerRuntime"` // SECLDoc[runtime] Definition:`Runtime managing the container`
}

// Hash returns a unique key for the entity
func (c *ContainerContext) Hash() string {
	return string(c.ContainerID)
}

// ParentScope returns the parent entity scope
func (c *ContainerContext) ParentScope() (eval.VariableScope, bool) {
	return nil, false
}

// SecurityProfileContext holds the security context of the profile
type SecurityProfileContext struct {
	Name           string                     `field:"name"`        // SECLDoc[name] Definition:`Name of the security profile`
	Version        string                     `field:"version"`     // SECLDoc[version] Definition:`Version of the security profile`
	Tags           []string                   `field:"tags"`        // SECLDoc[tags] Definition:`Tags of the security profile`
	EventTypes     []EventType                `field:"event_types"` // SECLDoc[event_types] Definition:`Event types enabled for the security profile`
	EventTypeState EventFilteringProfileState `field:"-"`           // State of the event type in this profile
}

// IPPortContext is used to hold an IP and Port
type IPPortContext struct {
	IPNet            net.IPNet `field:"ip"`                                               // SECLDoc[ip] Definition:`IP address`
	Port             uint16    `field:"port"`                                             // SECLDoc[port] Definition:`Port number`
	IsPublic         bool      `field:"is_public,handler:ResolveIsIPPublic,opts:skip_ad"` // SECLDoc[is_public] Definition:`Whether the IP address belongs to a public network`
	IsPublicResolved bool      `field:"-"`
}

// GetComparable returns a comparable version of IPPortContext
func (ipc *IPPortContext) GetComparable() netip.AddrPort {
	ipcAddr, ok := netip.AddrFromSlice(ipc.IPNet.IP)
	if !ok {
		return netip.AddrPort{}
	}
	return netip.AddrPortFrom(ipcAddr, ipc.Port)
}

// NetworkContext represents the network context of the event
type NetworkContext struct {
	Device NetworkDeviceContext `field:"device"` // network device on which the network packet was captured

	L3Protocol       uint16        `field:"l3_protocol"`       // SECLDoc[l3_protocol] Definition:`L3 protocol of the network packet` Constants:`L3 protocols`
	L4Protocol       uint16        `field:"l4_protocol"`       // SECLDoc[l4_protocol] Definition:`L4 protocol of the network packet` Constants:`L4 protocols`
	Source           IPPortContext `field:"source"`            // source of the network packet
	Destination      IPPortContext `field:"destination"`       // destination of the network packet
	NetworkDirection uint32        `field:"network_direction"` // SECLDoc[network_direction] Definition:`Network direction of the network packet` Constants:`Network directions`
	Size             uint32        `field:"size"`              // SECLDoc[size] Definition:`Size in bytes of the network packet`
}

// IsZero returns if there is a network context
func (nc *NetworkContext) IsZero() bool {
	return nc.Size == 0
}

// SpanContext describes a span context
type SpanContext struct {
	SpanID  uint64        `field:"-"`
	TraceID utils.TraceID `field:"-"`
}

// RuleContext defines a rule context
type RuleContext struct {
	Expression       string                `field:"-"`
	MatchingSubExprs eval.MatchingSubExprs `field:"-"`
}

// FileMetadata represents file metadata
type FileMetadata struct {
	Size               int64 `field:"size,handler:ResolveFileMetadataSize,opts:skip_ad,weight:999"`                               // SECLDoc[size] Definition:`[Experimental] Size of the file`
	Type               int   `field:"type,handler:ResolveFileMetadataType,opts:skip_ad,weight:999"`                               // SECLDoc[type] Definition:`[Experimental] Type of the file` Constants:`FileType`
	IsExecutable       bool  `field:"is_executable,handler:ResolveFileMetadataIsExecutable,opts:skip_ad,weight:999"`              // SECLDoc[is_executable] Definition:`[Experimental] Tells if the file is executable or not`
	Architecture       int   `field:"architecture,handler:ResolveFileMetadataArchitecture,opts:skip_ad,weight:999"`               // SECLDoc[architecture] Definition:`[Experimental] Architecture of the file (only for executable files)` Constants:`Architecture`
	ABI                int   `field:"abi,handler:ResolveFileMetadataABI,opts:skip_ad,weight:999"`                                 // SECLDoc[abi] Definition:`[Experimental] ABI of the file (only for executable files)` Constants:`ABI`
	IsUPXPacked        bool  `field:"is_upx_packed,handler:ResolveFileMetadataIsUPXPacked,opts:skip_ad,weight:999"`               // SECLDoc[is_upx_packed] Definition:`[Experimental] Tells if the binary has been packed using UPX`
	Compression        int   `field:"compression,handler:ResolveFileMetadataCompression,opts:skip_ad,weight:999"`                 // SECLDoc[compression] Definition:`[Experimental] Compression type of the file (only for compressed files)` Constants:`CompressionType`
	IsGarbleObfuscated bool  `field:"is_garble_obfuscated,handler:ResolveFileMetadataIsGarbleObfuscated,opts:skip_ad,weight:999"` // SECLDoc[is_garble_obfuscated] Definition:`[Experimental] Tells if the binary has been obfuscated using garble`
	Linkage            int   `field:"-"`
	Resolved           bool  `field:"-"`
}

// BaseEvent represents an event sent from the kernel
type BaseEvent struct {
	ID            string         `field:"-"`
	Type          uint32         `field:"-"`
	Flags         uint32         `field:"-"`
	TimestampRaw  uint64         `field:"event.timestamp,handler:ResolveEventTimestamp"` // SECLDoc[event.timestamp] Definition:`Timestamp of the event`
	Timestamp     time.Time      `field:"timestamp,opts:getters_only|gen_getters,handler:ResolveEventTime"`
	Rules         []*MatchedRule `field:"-"`
	RuleContext   RuleContext    `field:"-"`
	ActionReports []ActionReport `field:"-"`
	Os            string         `field:"event.os"`                                                      // SECLDoc[event.os] Definition:`Operating system of the event`
	Origin        string         `field:"event.origin"`                                                  // SECLDoc[event.origin] Definition:`Origin of the event`
	Service       string         `field:"event.service,handler:ResolveService,opts:skip_ad|gen_getters"` // SECLDoc[event.service] Definition:`Service associated with the event`
	Hostname      string         `field:"event.hostname,handler:ResolveHostname"`                        // SECLDoc[event.hostname] Definition:`Hostname associated with the event`
	RuleTags      []string       `field:"event.rule.tags"`                                               // SECLDoc[event.rule.tags] Definition:`Tags associated with the rule that's used to evaluate the event`

	// context shared with all event types
	ProcessContext         *ProcessContext        `field:"process"`
	ContainerContext       *ContainerContext      `field:"container"`
	SecurityProfileContext SecurityProfileContext `field:"-"`

	// internal usage
	PIDContext        PIDContext         `field:"-"`
	ProcessCacheEntry *ProcessCacheEntry `field:"-"`

	// mark event with having error
	Error error `field:"-"`

	// field resolution
	FieldHandlers FieldHandlers `field:"-"`
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

// Init initialize the event
func (e *Event) Init() {
	initMember(reflect.ValueOf(e).Elem(), map[string]bool{})
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

// HasActiveActivityDump returns true if the event has an active activity dump associated to it
func (e *Event) HasActiveActivityDump() bool {
	return e.Flags&EventFlagsHasActiveActivityDump > 0
}

// IsAnomalyDetectionEvent returns true if the current event is an anomaly detection event (kernel or user space)
func (e *Event) IsAnomalyDetectionEvent() bool {
	return e.Flags&EventFlagsAnomalyDetectionEvent > 0
}

// AddToFlags adds a flag to the event
func (e *Event) AddToFlags(flag uint32) {
	e.Flags |= flag
}

// ResetAnomalyDetectionEvent removes the anomaly detection event flag
func (e *Event) ResetAnomalyDetectionEvent() {
	if e.IsAnomalyDetectionEvent() {
		e.RemoveFromFlags(EventFlagsAnomalyDetectionEvent)
	}
}

// RemoveFromFlags remove a flag to the event
func (e *Event) RemoveFromFlags(flag uint32) {
	e.Flags ^= flag
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

// GetActionReports returns the triggred action reports
func (e *Event) GetActionReports() []ActionReport {
	return e.ActionReports
}

// GetWorkloadID returns an ID that represents the workload
func (e *Event) GetWorkloadID() string {
	return e.SecurityProfileContext.Name
}

// ResolveProcessCacheEntry uses the field handler
func (e *Event) ResolveProcessCacheEntry(newEntryCb func(*ProcessCacheEntry, error)) (*ProcessCacheEntry, bool) {
	return e.FieldHandlers.ResolveProcessCacheEntry(e, newEntryCb)
}

// ResolveEventTime uses the field handler
func (e *Event) ResolveEventTime() time.Time {
	return e.FieldHandlers.ResolveEventTime(e, &e.BaseEvent)
}

// ResolveService uses the field handler
func (e *Event) ResolveService() string {
	return e.FieldHandlers.ResolveService(e, &e.BaseEvent)
}

// UserSessionContext describes the user session context
// Disclaimer: the `json` tags are used to parse K8s credentials from cws-instrumentation
type UserSessionContext struct {
	ID          uint64           `field:"-"`
	SessionType usersession.Type `field:"-"`
	Resolved    bool             `field:"-"`
	// Kubernetes User Session context
	K8SUsername string              `field:"k8s_username,handler:ResolveK8SUsername" json:"username,omitempty"` // SECLDoc[k8s_username] Definition:`Kubernetes username of the user that executed the process`
	K8SUID      string              `field:"k8s_uid,handler:ResolveK8SUID" json:"uid,omitempty"`                // SECLDoc[k8s_uid] Definition:`Kubernetes UID of the user that executed the process`
	K8SGroups   []string            `field:"k8s_groups,handler:ResolveK8SGroups" json:"groups,omitempty"`       // SECLDoc[k8s_groups] Definition:`Kubernetes groups of the user that executed the process`
	K8SExtra    map[string][]string `json:"extra,omitempty"`
}

// MatchedRule contains the identification of one rule that has match
type MatchedRule struct {
	RuleID        string
	RuleVersion   string
	RuleTags      map[string]string
	PolicyName    string
	PolicyVersion string
}

// ActionReport defines an action report
type ActionReport interface {
	ToJSON() ([]byte, error)
	IsMatchingRule(ruleID eval.RuleID) bool
	IsResolved() error
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
	// HashFailed means that the hashing failed
	HashFailed
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
	// SSDEEP is used to identify a SSDEEP hash
	SSDEEP
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
	case SSDEEP:
		return "ssdeep"
	default:
		return ""
	}
}

var zeroProcessContext ProcessContext

// ProcessCacheEntry this struct holds process context kept in the process tree
type ProcessCacheEntry struct {
	ProcessContext

	refCount    uint64                     `field:"-"`
	coreRelease func(_ *ProcessCacheEntry) `field:"-"`
	onRelease   []func()                   `field:"-"`
}

// IsContainerRoot returns whether this is a top level process in the container ID
func (pc *ProcessCacheEntry) IsContainerRoot() bool {
	return pc.ContainerID != "" && pc.Ancestor != nil && pc.Ancestor.ContainerID == ""
}

// Reset the entry
func (pc *ProcessCacheEntry) Reset() {
	pc.ProcessContext = zeroProcessContext
	pc.refCount = 0
	// `coreRelease` function should not be cleared on reset
	// it's used for pool and cache size management
	pc.onRelease = nil
}

// Retain increment ref counter
func (pc *ProcessCacheEntry) Retain() {
	pc.refCount++
}

// AppendReleaseCallback set the callback called when the entry is released
func (pc *ProcessCacheEntry) AppendReleaseCallback(callback func()) {
	if callback != nil {
		pc.onRelease = append(pc.onRelease, callback)
	}
}

func (pc *ProcessCacheEntry) callReleaseCallbacks() {
	for _, cb := range pc.onRelease {
		cb()
	}
	if pc.coreRelease != nil {
		pc.coreRelease(pc)
	}
}

// Release decrement and eventually release the entry
func (pc *ProcessCacheEntry) Release() {
	if pc.refCount > 1 {
		pc.refCount--
		return
	}

	pc.callReleaseCallbacks()
}

// NewProcessCacheEntry returns a new process cache entry
func NewProcessCacheEntry(coreRelease func(_ *ProcessCacheEntry)) *ProcessCacheEntry {
	return &ProcessCacheEntry{coreRelease: coreRelease}
}

// ProcessAncestorsIterator defines an iterator of ancestors
type ProcessAncestorsIterator struct {
	Root *ProcessCacheEntry
	prev *ProcessCacheEntry
}

// Front returns the first element
func (it *ProcessAncestorsIterator) Front(_ *eval.Context) *ProcessCacheEntry {
	if it.Root != nil {
		it.prev = it.Root
	}
	return it.prev
}

// Next returns the next element
func (it *ProcessAncestorsIterator) Next(_ *eval.Context) *ProcessCacheEntry {
	if next := it.prev.Ancestor; next != nil {
		it.prev = next
		return next
	}

	return nil
}

// At returns the element at the given position
func (it *ProcessAncestorsIterator) At(ctx *eval.Context, regID eval.RegisterID, pos int) *ProcessCacheEntry {
	if entry := ctx.RegisterCache[regID]; entry != nil && entry.Pos == pos {
		return entry.Value.(*ProcessCacheEntry)
	}

	var i int

	ancestor := ctx.Event.(*Event).ProcessContext.Ancestor
	for ancestor != nil {
		if i == pos {
			ctx.RegisterCache[regID] = &eval.RegisterCacheEntry{
				Pos:   pos,
				Value: ancestor,
			}
			return ancestor
		}
		ancestor = ancestor.Ancestor
		i++
	}

	return nil
}

// Len returns the len
func (it *ProcessAncestorsIterator) Len(ctx *eval.Context) int {
	var size int

	ancestor := ctx.Event.(*Event).ProcessContext.Ancestor
	for ancestor != nil {
		size++
		ancestor = ancestor.Ancestor
	}

	return size
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

// HasResponse returns whether the DNS event has a response
func (de *DNSEvent) HasResponse() bool {
	return de.Response != nil
}

// ExitEvent represents a process exit event
type ExitEvent struct {
	*Process
	Cause uint32 `field:"cause"`                 // SECLDoc[cause] Definition:`Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)`
	Code  uint32 `field:"code,opts:gen_getters"` // SECLDoc[code] Definition:`Exit code of the process or number of the signal that caused the process to terminate`
}

// DNSQuestion represents the dns question
type DNSQuestion struct {
	Name  string `field:"name,opts:length" op_override:"eval.CaseInsensitiveCmp"` // SECLDoc[name] Definition:`the queried domain name`
	Type  uint16 `field:"type"`                                                   // SECLDoc[type] Definition:`a two octet code which specifies the DNS question type` Constants:`DNS qtypes`
	Class uint16 `field:"class"`                                                  // SECLDoc[class] Definition:`the class looked up by the DNS question` Constants:`DNS qclasses`
	Size  uint16 `field:"length"`                                                 // SECLDoc[length] Definition:`the total DNS request size in bytes`
	Count uint16 `field:"count"`                                                  // SECLDoc[count] Definition:`the total count of questions in the DNS request`
}

// DNSEvent represents a DNS request event
type DNSEvent struct {
	ID       uint16       `field:"id"` // SECLDoc[id] Definition:`[Experimental] the DNS request ID`
	Question DNSQuestion  `field:"question"`
	Response *DNSResponse `field:"response,check:HasResponse"`
}

// DNSResponse represents a DNS response event
type DNSResponse struct {
	ResponseCode uint8 `field:"code"` // SECLDoc[code] Definition:`Response code of the DNS response according to RFC 1035` Constants:`DNS Responses`
}

// Matches returns true if the two DNS events matches
func (de *DNSEvent) Matches(new *DNSEvent) bool {
	return de.Question.Name == new.Question.Name && de.Question.Type == new.Question.Type && de.Question.Class == new.Question.Class
}

// IMDSEvent represents an IMDS event
type IMDSEvent struct {
	Type          string `field:"type"`           // SECLDoc[type] Definition:`the type of IMDS event`
	CloudProvider string `field:"cloud_provider"` // SECLDoc[cloud_provider] Definition:`the intended cloud provider of the IMDS event`
	URL           string `field:"url"`            // SECLDoc[url] Definition:`the queried IMDS URL`
	Host          string `field:"host"`           // SECLDoc[host] Definition:`the host of the HTTP protocol`
	UserAgent     string `field:"user_agent"`     // SECLDoc[user_agent] Definition:`the user agent of the HTTP client`
	Server        string `field:"server"`         // SECLDoc[server] Definition:`the server header of a response`

	// The fields below are optional and cloud specific fields
	AWS AWSIMDSEvent `field:"aws"` // SECLDoc[aws] Definition:`the AWS specific data parsed from the IMDS event`
}

// AWSIMDSEvent holds data from an AWS IMDS event
type AWSIMDSEvent struct {
	IsIMDSv2            bool                   `field:"is_imds_v2"`           // SECLDoc[is_imds_v2] Definition:`a boolean which specifies if the IMDS event follows IMDSv1 or IMDSv2 conventions`
	SecurityCredentials AWSSecurityCredentials `field:"security_credentials"` // SECLDoc[credentials] Definition:`the security credentials in the IMDS answer`
}

// AWSSecurityCredentials is used to parse the fields that are none to be free of credentials or secrets
type AWSSecurityCredentials struct {
	Code        string    `field:"-" json:"Code"`
	Type        string    `field:"type" json:"Type"` // SECLDoc[type] Definition:`the security credentials type`
	AccessKeyID string    `field:"-" json:"AccessKeyId"`
	LastUpdated string    `field:"-" json:"LastUpdated"`
	Expiration  time.Time `field:"-"`

	ExpirationRaw string `field:"-" json:"Expiration"`
}

// BaseExtraFieldHandlers handlers not hold by any field
type BaseExtraFieldHandlers interface {
	ResolveProcessCacheEntry(ev *Event, newEntryCb func(*ProcessCacheEntry, error)) (*ProcessCacheEntry, bool)
	ResolveProcessCacheEntryFromPID(pid uint32) *ProcessCacheEntry
	ResolveContainerContext(ev *Event) (*ContainerContext, bool)
}

// ResolveProcessCacheEntry stub implementation
func (dfh *FakeFieldHandlers) ResolveProcessCacheEntry(ev *Event, _ func(*ProcessCacheEntry, error)) (*ProcessCacheEntry, bool) {
	if ev.ProcessCacheEntry != nil {
		return ev.ProcessCacheEntry, true
	}
	return nil, false
}

// ResolveContainerContext stub implementation
func (dfh *FakeFieldHandlers) ResolveContainerContext(_ *Event) (*ContainerContext, bool) {
	return nil, false
}

// TLSContext represents a tls context
type TLSContext struct {
	Version uint16 `field:"version"` // SECLDoc[version] Definition:`TLS version`
}
