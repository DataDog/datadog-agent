// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors -tags linux -output accessors_linux.go -field-handlers-tags unix -field-handlers field_handlers_unix.go -doc ../../../../docs/cloud-workload-security/secl.json
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors -tags windows -output accessors_windows.go -field-handlers-tags windows -field-handlers field_handlers_windows.go

package model

import (
	"errors"
	"fmt"
	"net"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

const (
	// OverlayFS overlay filesystem
	OverlayFS = "overlay"
)

// Model describes the data model for the runtime security agent events
type Model struct {
	ExtraValidateFieldFnc func(field eval.Field, fieldValue eval.FieldValue) error
}

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{}
}

// NewDefaultEventWithType returns a new Event for the given type
func (m *Model) NewDefaultEventWithType(kind EventType) eval.Event {
	return &Event{
		Type:          uint32(kind),
		FieldHandlers: &DefaultFieldHandlers{},
	}
}

// check that all path are absolute
func validatePath(field eval.Field, fieldValue eval.FieldValue) error {
	// do not support regular expression on path, currently unable to support discarder for regex value
	if fieldValue.Type == eval.RegexpValueType {
		return fmt.Errorf("regexp not supported on path `%s`", field)
	} else if fieldValue.Type == eval.VariableValueType {
		return nil
	}

	if value, ok := fieldValue.Value.(string); ok {
		errAbs := fmt.Errorf("invalid path `%s`, all the path have to be absolute", value)
		errDepth := fmt.Errorf("invalid path `%s`, path depths have to be shorter than %d", value, MaxPathDepth)
		errSegment := fmt.Errorf("invalid path `%s`, each segment of a path must be shorter than %d", value, MaxSegmentLength)

		if value == "" {
			return nil
		}

		if value != path.Clean(value) {
			return errAbs
		}

		if value == "*" {
			return errAbs
		}

		if !filepath.IsAbs(value) && len(value) > 0 && value[0] != '*' {
			return errAbs
		}

		if strings.HasPrefix(value, "~") {
			return errAbs
		}

		// check resolution limitations
		segments := strings.Split(value, "/")
		if len(segments) > MaxPathDepth {
			return errDepth
		}
		for _, segment := range segments {
			if segment == ".." {
				return errAbs
			}
			if len(segment) > MaxSegmentLength {
				return errSegment
			}
		}
	}

	return nil
}

// ValidateField validates the value of a field
func (m *Model) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	if strings.HasSuffix(field, "path") {
		if err := validatePath(field, fieldValue); err != nil {
			return err
		}
	}

	switch field {

	case "event.retval":
		if value := fieldValue.Value; value != -int(syscall.EPERM) && value != -int(syscall.EACCES) {
			return errors.New("return value can only be tested against EPERM or EACCES")
		}
	case "bpf.map.name", "bpf.prog.name":
		if value, ok := fieldValue.Value.(string); ok {
			if len(value) > MaxBpfObjName {
				return fmt.Errorf("the name provided in %s must be at most %d characters, len(\"%s\") = %d", field, MaxBpfObjName, value, len(value))
			}
		}
	}

	if m.ExtraValidateFieldFnc != nil {
		return m.ExtraValidateFieldFnc(field, fieldValue)
	}

	return nil
}

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode; file.destination.rights"` // SECLDoc[file.destination.mode] Definition:`New mode of the chmod-ed file` Constants:`Chmod mode constants` SECLDoc[file.destination.rights] Definition:`New rights of the chmod-ed file` Constants:`Chmod mode constants`
}

// ChownEvent represents a chown event
type ChownEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	UID   int64     `field:"file.destination.uid"`                           // SECLDoc[file.destination.uid] Definition:`New UID of the chown-ed file's owner`
	User  string    `field:"file.destination.user,handler:ResolveChownUID"`  // SECLDoc[file.destination.user] Definition:`New user of the chown-ed file's owner`
	GID   int64     `field:"file.destination.gid"`                           // SECLDoc[file.destination.gid] Definition:`New GID of the chown-ed file's owner`
	Group string    `field:"file.destination.group,handler:ResolveChownGID"` // SECLDoc[file.destination.group] Definition:`New group of the chown-ed file's owner`
}

// ContainerContext holds the container context of an event
type ContainerContext struct {
	ID        string   `field:"id,handler:ResolveContainerID"`                              // SECLDoc[id] Definition:`ID of the container`
	CreatedAt uint64   `field:"created_at,handler:ResolveContainerCreatedAt"`               // SECLDoc[created_at] Definition:`Timestamp of the creation of the container``
	Tags      []string `field:"tags,handler:ResolveContainerTags,opts:skip_ad,weight:9999"` // SECLDoc[tags] Definition:`Tags of the container`
}

type Status uint32

const (
	// AnomalyDetection will trigger alerts each time an event is not part of the profile
	AnomalyDetection Status = 1 << iota
	// AutoSuppression will suppress any signal to events present on the profile
	AutoSuppression
	// WorkloadHardening will kill the process that triggered anomaly detection
	WorkloadHardening
)

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
	Name    string   `field:"name"`    // SECLDoc[name] Definition:`Name of the security profile`
	Status  Status   `field:"status"`  // SECLDoc[status] Definition:`Status of the security profile`
	Version string   `field:"version"` // SECLDoc[version] Definition:`Version of the security profile`
	Tags    []string `field:"tags"`    // SECLDoc[tags] Definition:`Tags of the security profile`
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	ID           string         `field:"-" json:"-"`
	Type         uint32         `field:"-"`
	Flags        uint32         `field:"-"`
	Async        bool           `field:"async,handler:ResolveAsync" event:"*" platform:"linux"` // SECLDoc[async] Definition:`True if the syscall was asynchronous`
	TimestampRaw uint64         `field:"-" json:"-"`
	Timestamp    time.Time      `field:"-"` // Timestamp of the event
	Rules        []*MatchedRule `field:"-"`

	// context shared with all events
	ProcessCacheEntry      *ProcessCacheEntry     `field:"-" json:"-" platform:"linux"`
	PIDContext             PIDContext             `field:"-" json:"-" platform:"linux"`
	SpanContext            SpanContext            `field:"-" json:"-" platform:"linux"`
	ProcessContext         *ProcessContext        `field:"process" event:"*" platform:"linux"`
	ContainerContext       ContainerContext       `field:"container" platform:"linux"`
	NetworkContext         NetworkContext         `field:"network" platform:"linux"`
	SecurityProfileContext SecurityProfileContext `field:"-"`

	// fim events
	Chmod       ChmodEvent    `field:"chmod" event:"chmod" platform:"linux"`             // [7.27] [File] A file’s permissions were changed
	Chown       ChownEvent    `field:"chown" event:"chown" platform:"linux"`             // [7.27] [File] A file’s owner was changed
	Open        OpenEvent     `field:"open" event:"open" platform:"linux"`               // [7.27] [File] A file was opened
	Mkdir       MkdirEvent    `field:"mkdir" event:"mkdir" platform:"linux"`             // [7.27] [File] A directory was created
	Rmdir       RmdirEvent    `field:"rmdir" event:"rmdir" platform:"linux"`             // [7.27] [File] A directory was removed
	Rename      RenameEvent   `field:"rename" event:"rename" platform:"linux"`           // [7.27] [File] A file/directory was renamed
	Unlink      UnlinkEvent   `field:"unlink" event:"unlink" platform:"linux"`           // [7.27] [File] A file was deleted
	Utimes      UtimesEvent   `field:"utimes" event:"utimes" platform:"linux"`           // [7.27] [File] Change file access/modification times
	Link        LinkEvent     `field:"link" event:"link" platform:"linux"`               // [7.27] [File] Create a new name/alias for a file
	SetXAttr    SetXAttrEvent `field:"setxattr" event:"setxattr" platform:"linux"`       // [7.27] [File] Set exteneded attributes
	RemoveXAttr SetXAttrEvent `field:"removexattr" event:"removexattr" platform:"linux"` // [7.27] [File] Remove extended attributes
	Splice      SpliceEvent   `field:"splice" event:"splice" platform:"linux"`           // [7.36] [File] A splice command was executed
	Mount       MountEvent    `field:"mount" event:"mount" platform:"linux"`             // [7.42] [File] [Experimental] A filesystem was mounted

	// process events
	Exec     ExecEvent     `field:"exec" event:"exec" platform:"linux"`     // [7.27] [Process] A process was executed or forked
	SetUID   SetuidEvent   `field:"setuid" event:"setuid" platform:"linux"` // [7.27] [Process] A process changed its effective uid
	SetGID   SetgidEvent   `field:"setgid" event:"setgid" platform:"linux"` // [7.27] [Process] A process changed its effective gid
	Capset   CapsetEvent   `field:"capset" event:"capset" platform:"linux"` // [7.27] [Process] A process changed its capacity set
	Signal   SignalEvent   `field:"signal" event:"signal" platform:"linux"` // [7.35] [Process] A signal was sent
	Exit     ExitEvent     `field:"exit" event:"exit" platform:"linux"`     // [7.38] [Process] A process was terminated
	Syscalls SyscallsEvent `field:"-" platform:"linux"`

	// anomaly detection related events
	AnomalyDetectionSyscallEvent AnomalyDetectionSyscallEvent `field:"-"`

	// kernel events
	SELinux      SELinuxEvent      `field:"selinux" event:"selinux" platform:"linux"`             // [7.30] [Kernel] An SELinux operation was run
	BPF          BPFEvent          `field:"bpf" event:"bpf" platform:"linux"`                     // [7.33] [Kernel] A BPF command was executed
	PTrace       PTraceEvent       `field:"ptrace" event:"ptrace" platform:"linux"`               // [7.35] [Kernel] A ptrace command was executed
	MMap         MMapEvent         `field:"mmap" event:"mmap" platform:"linux"`                   // [7.35] [Kernel] A mmap command was executed
	MProtect     MProtectEvent     `field:"mprotect" event:"mprotect" platform:"linux"`           // [7.35] [Kernel] A mprotect command was executed
	LoadModule   LoadModuleEvent   `field:"load_module" event:"load_module" platform:"linux"`     // [7.35] [Kernel] A new kernel module was loaded
	UnloadModule UnloadModuleEvent `field:"unload_module" event:"unload_module" platform:"linux"` // [7.35] [Kernel] A kernel module was deleted

	// network events
	DNS  DNSEvent  `field:"dns" event:"dns" platform:"linux"`   // [7.36] [Network] A DNS request was sent
	Bind BindEvent `field:"bind" event:"bind" platform:"linux"` // [7.37] [Network] [Experimental] A bind was executed

	// internal usage
	Umount              UmountEvent           `field:"-" json:"-" platform:"linux"`
	InvalidateDentry    InvalidateDentryEvent `field:"-" json:"-" platform:"linux"`
	ArgsEnvs            ArgsEnvsEvent         `field:"-" json:"-" platform:"linux"`
	MountReleased       MountReleasedEvent    `field:"-" json:"-" platform:"linux"`
	CgroupTracing       CgroupTracingEvent    `field:"-" json:"-" platform:"linux"`
	NetDevice           NetDeviceEvent        `field:"-" json:"-" platform:"linux"`
	VethPair            VethPairEvent         `field:"-" json:"-" platform:"linux"`
	UnshareMountNS      UnshareMountNSEvent   `field:"-" json:"-" platform:"linux"`
	PathResolutionError error                 `field:"-" json:"-" platform:"linux"` // hold one of the path resolution error

	// field resolution
	FieldHandlers FieldHandlers `field:"-" json:"-" platform:"linux"`
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
func NewDefaultEvent() eval.Event {
	return &Event{
		FieldHandlers: &DefaultFieldHandlers{},
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

// IsSavedByActivityDumps return whether AD sample
func (e *Event) IsActivityDumpSample() bool {
	return e.Flags&EventFlagsActivityDumpSample > 0
}

// IsInProfile return true if the event was fount in the profile
func (e *Event) IsInProfile() bool {
	return e.Flags&EventFlagsSecurityProfileInProfile > 0
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

// Retain the event
func (ev *Event) Retain() Event {
	if ev.ProcessCacheEntry != nil {
		ev.ProcessCacheEntry.Retain()
	}
	return *ev
}

// Release the event
func (ev *Event) Release() {
	if ev.ProcessCacheEntry != nil {
		ev.ProcessCacheEntry.Release()
	}
}

// SetPathResolutionError sets the Event.pathResolutionError
func (ev *Event) SetPathResolutionError(fileFields *FileEvent, err error) {
	fileFields.PathResolutionError = err
	ev.PathResolutionError = err
}

// ResolveProcessCacheEntry uses the field handler
func (ev *Event) ResolveProcessCacheEntry() (*ProcessCacheEntry, bool) {
	return ev.FieldHandlers.ResolveProcessCacheEntry(ev)
}

// ResolveEventTimestamp uses the field handler
func (ev *Event) ResolveEventTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTimestamp(ev)
}

// GetProcessServiceTag uses the field handler
func (ev *Event) GetProcessServiceTag() string {
	return ev.FieldHandlers.GetProcessServiceTag(ev)
}

// MatchedRules contains the identification of one rule that has match
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

// Append two lists, but avoiding duplicates
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

// SetuidEvent represents a setuid event
type SetuidEvent struct {
	UID    uint32 `field:"uid"`                                // SECLDoc[uid] Definition:`New UID of the process`
	User   string `field:"user,handler:ResolveSetuidUser"`     // SECLDoc[user] Definition:`New user of the process`
	EUID   uint32 `field:"euid"`                               // SECLDoc[euid] Definition:`New effective UID of the process`
	EUser  string `field:"euser,handler:ResolveSetuidEUser"`   // SECLDoc[euser] Definition:`New effective user of the process`
	FSUID  uint32 `field:"fsuid"`                              // SECLDoc[fsuid] Definition:`New FileSystem UID of the process`
	FSUser string `field:"fsuser,handler:ResolveSetuidFSUser"` // SECLDoc[fsuser] Definition:`New FileSystem user of the process`
}

// SetgidEvent represents a setgid event
type SetgidEvent struct {
	GID     uint32 `field:"gid"`                                  // SECLDoc[gid] Definition:`New GID of the process`
	Group   string `field:"group,handler:ResolveSetgidGroup"`     // SECLDoc[group] Definition:`New group of the process`
	EGID    uint32 `field:"egid"`                                 // SECLDoc[egid] Definition:`New effective GID of the process`
	EGroup  string `field:"egroup,handler:ResolveSetgidEGroup"`   // SECLDoc[egroup] Definition:`New effective group of the process`
	FSGID   uint32 `field:"fsgid"`                                // SECLDoc[fsgid] Definition:`New FileSystem GID of the process`
	FSGroup string `field:"fsgroup,handler:ResolveSetgidFSGroup"` // SECLDoc[fsgroup] Definition:`New FileSystem group of the process`
}

// CapsetEvent represents a capset event
type CapsetEvent struct {
	CapEffective uint64 `field:"cap_effective"` // SECLDoc[cap_effective] Definition:`Effective capability set of the process` Constants:`Kernel Capability constants`
	CapPermitted uint64 `field:"cap_permitted"` // SECLDoc[cap_permitted] Definition:`Permitted capability set of the process` Constants:`Kernel Capability constants`
}

// Credentials represents the kernel credentials of a process
type Credentials struct {
	UID   uint32 `field:"uid"`   // SECLDoc[uid] Definition:`UID of the process`
	GID   uint32 `field:"gid"`   // SECLDoc[gid] Definition:`GID of the process`
	User  string `field:"user"`  // SECLDoc[user] Definition:`User of the process` Example:`process.user == "root"` Description:`Constrain an event to be triggered by a process running as the root user.`
	Group string `field:"group"` // SECLDoc[group] Definition:`Group of the process`

	EUID   uint32 `field:"euid"`   // SECLDoc[euid] Definition:`Effective UID of the process`
	EGID   uint32 `field:"egid"`   // SECLDoc[egid] Definition:`Effective GID of the process`
	EUser  string `field:"euser"`  // SECLDoc[euser] Definition:`Effective user of the process`
	EGroup string `field:"egroup"` // SECLDoc[egroup] Definition:`Effective group of the process`

	FSUID   uint32 `field:"fsuid"`   // SECLDoc[fsuid] Definition:`FileSystem-uid of the process`
	FSGID   uint32 `field:"fsgid"`   // SECLDoc[fsgid] Definition:`FileSystem-gid of the process`
	FSUser  string `field:"fsuser"`  // SECLDoc[fsuser] Definition:`FileSystem-user of the process`
	FSGroup string `field:"fsgroup"` // SECLDoc[fsgroup] Definition:`FileSystem-group of the process`

	CapEffective uint64 `field:"cap_effective"` // SECLDoc[cap_effective] Definition:`Effective capability set of the process` Constants:`Kernel Capability constants`
	CapPermitted uint64 `field:"cap_permitted"` // SECLDoc[cap_permitted] Definition:`Permitted capability set of the process` Constants:`Kernel Capability constants`
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (p *Process) GetPathResolutionError() string {
	return p.FileEvent.GetPathResolutionError()
}

// HasInterpreter returns whether the process uses an interpreter
func (p *Process) HasInterpreter() bool {
	return p.LinuxBinprm.FileEvent.Inode != 0
}

// IsNotKworker returns true if the process isn't a kworker
func (p *Process) IsNotKworker() bool {
	return !p.IsKworker
}

// LinuxBinprm contains content from the linux_binprm struct, which holds the arguments used for loading binaries
type LinuxBinprm struct {
	FileEvent FileEvent `field:"file"`
}

// Process represents a process
type Process struct {
	PIDContext

	FileEvent FileEvent `field:"file,check:IsNotKworker"`

	ContainerID string `field:"container.id"` // SECLDoc[container.id] Definition:`Container ID`

	SpanID  uint64 `field:"-"`
	TraceID uint64 `field:"-"`

	TTYName     string      `field:"tty_name"`                         // SECLDoc[tty_name] Definition:`Name of the TTY associated with the process`
	Comm        string      `field:"comm"`                             // SECLDoc[comm] Definition:`Comm attribute of the process`
	LinuxBinprm LinuxBinprm `field:"interpreter,check:HasInterpreter"` // Script interpreter as identified by the shebang

	// pid_cache_t
	ForkTime time.Time `field:"-" json:"-"`
	ExitTime time.Time `field:"-" json:"-"`
	ExecTime time.Time `field:"-" json:"-"`

	CreatedAt uint64 `field:"created_at,handler:ResolveProcessCreatedAt"` // SECLDoc[created_at] Definition:`Timestamp of the creation of the process`

	Cookie uint32 `field:"-"`
	PPid   uint32 `field:"ppid"` // SECLDoc[ppid] Definition:`Parent process ID`

	// credentials_t section of pid_cache_t
	Credentials ``

	ArgsID uint32 `field:"-" json:"-"`
	EnvsID uint32 `field:"-" json:"-"`

	ArgsEntry *ArgsEntry `field:"-" json:"-"`
	EnvsEntry *EnvsEntry `field:"-" json:"-"`

	// defined to generate accessors, ArgsTruncated and EnvsTruncated are used during by unmarshaller
	Argv0         string   `field:"argv0,handler:ResolveProcessArgv0,weight:100"`                                                                                                                                               // SECLDoc[argv0] Definition:`First argument of the process`
	Args          string   `field:"args,handler:ResolveProcessArgs,weight:100"`                                                                                                                                                 // SECLDoc[args] Definition:`Arguments of the process (as a string, excluding argv0)` Example:`exec.args == "-sV -p 22,53,110,143,4564 198.116.0-255.1-127"` Description:`Matches any process with these exact arguments.` Example:`exec.args =~ "* -F * http*"` Description:`Matches any process that has the "-F" argument anywhere before an argument starting with "http".`
	Argv          []string `field:"argv,handler:ResolveProcessArgv,weight:100; args_flags,handler:ResolveProcessArgsFlags,opts:cacheless_resolution; args_options,handler:ResolveProcessArgsOptions,opts:cacheless_resolution"` // SECLDoc[argv] Definition:`Arguments of the process (as an array, excluding argv0)` Example:`exec.argv in ["127.0.0.1"]` Description:`Matches any process that has this IP address as one of its arguments.` SECLDoc[args_flags] Definition:`Flags in the process arguments` Example:`exec.args_flags in ["s"] && exec.args_flags in ["V"]` Description:`Matches any process with both "-s" and "-V" flags in its arguments. Also matches "-sV".` SECLDoc[args_options] Definition:`Argument of the process as options` Example:`exec.args_options in ["p=0-1024"]` Description:`Matches any process that has either "-p 0-1024" or "--p=0-1024" in its arguments.`
	ArgsTruncated bool     `field:"args_truncated,handler:ResolveProcessArgsTruncated"`                                                                                                                                         // SECLDoc[args_truncated] Definition:`Indicator of arguments truncation`
	Envs          []string `field:"envs,handler:ResolveProcessEnvs:100"`                                                                                                                                                        // SECLDoc[envs] Definition:`Environment variable names of the process`
	Envp          []string `field:"envp,handler:ResolveProcessEnvp:100"`                                                                                                                                                        // SECLDoc[envp] Definition:`Environment variables of the process`
	EnvsTruncated bool     `field:"envs_truncated,handler:ResolveProcessEnvsTruncated"`                                                                                                                                         // SECLDoc[envs_truncated] Definition:`Indicator of environment variables truncation`

	// symlink to the process binary
	SymlinkPathnameStr [MaxSymlinks]string `field:"-" json:"-"`
	SymlinkBasenameStr string              `field:"-" json:"-"`

	// cache version
	ScrubbedArgvResolved  bool           `field:"-" json:"-"`
	ScrubbedArgv          []string       `field:"-" json:"-"`
	ScrubbedArgsTruncated bool           `field:"-" json:"-"`
	Variables             eval.Variables `field:"-" json:"-"`

	IsThread bool `field:"is_thread"` // SECLDoc[is_thread] Definition:`Indicates whether the process is considered a thread (that is, a child process that hasn't executed another program)`
}

// SpanContext describes a span context
type SpanContext struct {
	SpanID  uint64 `field:"_" json:"-"`
	TraceID uint64 `field:"_" json:"-"`
}

// ExecEvent represents a exec event
type ExecEvent struct {
	*Process
}

// ExitEvent represents a process exit event
type ExitEvent struct {
	*Process
	Cause uint32 `field:"cause"` // SECLDoc[cause] Definition:`Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)`
	Code  uint32 `field:"code"`  // SECLDoc[code] Definition:`Exit code of the process or number of the signal that caused the process to terminate`
}

// FileFields holds the information required to identify a file
type FileFields struct {
	UID   uint32 `field:"uid"`                                                         // SECLDoc[uid] Definition:`UID of the file's owner`
	User  string `field:"user,handler:ResolveFileFieldsUser"`                          // SECLDoc[user] Definition:`User of the file's owner`
	GID   uint32 `field:"gid"`                                                         // SECLDoc[gid] Definition:`GID of the file's owner`
	Group string `field:"group,handler:ResolveFileFieldsGroup"`                        // SECLDoc[group] Definition:`Group of the file's owner`
	Mode  uint16 `field:"mode;rights,handler:ResolveRights,opts:cacheless_resolution"` // SECLDoc[mode] Definition:`Mode of the file` SECLDoc[rights] Definition:`Rights of the file` Constants:`Chmod mode constants`
	CTime uint64 `field:"change_time"`                                                 // SECLDoc[change_time] Definition:`Change time of the file`
	MTime uint64 `field:"modification_time"`                                           // SECLDoc[modification_time] Definition:`Modification time of the file`

	PathKey
	InUpperLayer bool `field:"in_upper_layer,handler:ResolveFileFieldsInUpperLayer"` // SECLDoc[in_upper_layer] Definition:`Indicator of the file layer, for example, in an OverlayFS`

	NLink uint32 `field:"-" json:"-"`
	Flags int32  `field:"-" json:"-"`
}

// IsFileless return whether it is a file less access
func (f *FileFields) IsFileless() bool {
	// TODO(safchain) fix this heuristic by add a flag in the event intead of using mount ID 0
	return f.Inode != 0 && f.MountID == 0
}

// HasHardLinks returns whether the file has hardlink
func (f *FileFields) HasHardLinks() bool {
	return f.NLink > 1
}

// GetInLowerLayer returns whether a file is in a lower layer
func (f *FileFields) GetInLowerLayer() bool {
	return f.Flags&LowerLayer != 0
}

// GetInUpperLayer returns whether a file is in the upper layer
func (f *FileFields) GetInUpperLayer() bool {
	return f.Flags&UpperLayer != 0
}

// FileEvent is the common file event type
type FileEvent struct {
	FileFields ``

	PathnameStr string `field:"path,handler:ResolveFilePath,opts:length" op_override:"ProcessSymlinkPathname"`     // SECLDoc[path] Definition:`File's path` Example:`exec.file.path == "/usr/bin/apt"` Description:`Matches the execution of the file located at /usr/bin/apt` Example:`open.file.path == "/etc/passwd"` Description:`Matches any process opening the /etc/passwd file.`
	BasenameStr string `field:"name,handler:ResolveFileBasename,opts:length" op_override:"ProcessSymlinkBasename"` // SECLDoc[name] Definition:`File's basename` Example:`exec.file.name == "apt"` Description:`Matches the execution of any file named apt.`
	Filesystem  string `field:"filesystem,handler:ResolveFileFilesystem"`                                          // SECLDoc[filesystem] Definition:`File's filesystem`

	PathResolutionError error `field:"-" json:"-"`

	PkgName       string `field:"package.name,handler:ResolvePackageName"`                    // SECLDoc[package.name] Definition:`[Experimental] Name of the package that provided this file`
	PkgVersion    string `field:"package.version,handler:ResolvePackageVersion"`              // SECLDoc[package.version] Definition:`[Experimental] Full version of the package that provided this file`
	PkgSrcVersion string `field:"package.source_version,handler:ResolvePackageSourceVersion"` // SECLDoc[package.source_version] Definition:`[Experimental] Full version of the source package of the package that provided this file`

	// used to mark as already resolved, can be used in case of empty path
	IsPathnameStrResolved bool `field:"-" json:"-"`
	IsBasenameStrResolved bool `field:"-" json:"-"`
}

// SetPathnameStr set and mark as resolved
func (e *FileEvent) SetPathnameStr(str string) {
	e.PathnameStr = str
	e.IsPathnameStrResolved = true
}

// SetBasenameStr set and mark as resolved
func (e *FileEvent) SetBasenameStr(str string) {
	e.BasenameStr = str
	e.IsBasenameStrResolved = true
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (e *FileEvent) GetPathResolutionError() string {
	if e.PathResolutionError != nil {
		return e.PathResolutionError.Error()
	}
	return ""
}

// InvalidateDentryEvent defines a invalidate dentry event
type InvalidateDentryEvent struct {
	Inode   uint64
	MountID uint32
}

// MountReleasedEvent defines a mount released event
type MountReleasedEvent struct {
	MountID uint32
}

// LinkEvent represents a link event
type LinkEvent struct {
	SyscallEvent
	Source FileEvent `field:"file"`
	Target FileEvent `field:"file.destination"`
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode; file.destination.rights"` // SECLDoc[file.destination.mode] Definition:`Mode of the new directory` Constants:`Chmod mode constants` SECLDoc[file.destination.rights] Definition:`Rights of the new directory` Constants:`Chmod mode constants`
}

// ArgsEnvsEvent defines a args/envs event
type ArgsEnvsEvent struct {
	ArgsEnvs
}

// Mount represents a mountpoint (used by MountEvent and UnshareMountNSEvent)
type Mount struct {
	MountID        uint32 `field:"-"`
	GroupID        uint32 `field:"-"`
	Device         uint32 `field:"-"`
	ParentMountID  uint32 `field:"-"`
	ParentInode    uint64 `field:"-"`
	RootMountID    uint32 `field:"-"`
	RootInode      uint64 `field:"-"`
	BindSrcMountID uint32 `field:"-"`
	FSType         string `field:"fs_type"` // SECLDoc[fs_type] Definition:`Type of the mounted file system`
	MountPointStr  string `field:"-"`
	RootStr        string `field:"-"`
	Path           string `field:"-"`
}

// MountEvent represents a mount event
//
//msgp:ignore MountEvent
type MountEvent struct {
	SyscallEvent
	Mount
	MountPointPath                 string `field:"mountpoint.path,handler:ResolveMountPointPath"` // SECLDoc[mountpoint.path] Definition:`Path of the mount point`
	MountSourcePath                string `field:"source.path,handler:ResolveMountSourcePath"`    // SECLDoc[source.path] Definition:`Source path of a bind mount`
	MountPointPathResolutionError  error  `field:"-"`
	MountSourcePathResolutionError error  `field:"-"`
}

// UnshareMountNSEvent represents a mount cloned from a newly created mount namespace
type UnshareMountNSEvent struct {
	Mount
}

// GetFSType returns the filesystem type of the mountpoint
func (m *Mount) GetFSType() string {
	return m.FSType
}

// IsOverlayFS returns whether it is an overlay fs
func (m *Mount) IsOverlayFS() bool {
	return m.GetFSType() == "overlay"
}

// OpenEvent represents an open event
type OpenEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	Flags uint32    `field:"flags"`                 // SECLDoc[flags] Definition:`Flags used when opening the file` Constants:`Open flags`
	Mode  uint32    `field:"file.destination.mode"` // SECLDoc[file.destination.mode] Definition:`Mode of the created file` Constants:`Chmod mode constants`
}

// SELinuxEventKind represents the event kind for SELinux events
type SELinuxEventKind uint32

const (
	// SELinuxBoolChangeEventKind represents SELinux boolean change events
	SELinuxBoolChangeEventKind SELinuxEventKind = iota
	// SELinuxStatusChangeEventKind represents SELinux status change events
	SELinuxStatusChangeEventKind
	// SELinuxBoolCommitEventKind represents SELinux boolean commit events
	SELinuxBoolCommitEventKind
)

// SELinuxEvent represents a selinux event
type SELinuxEvent struct {
	File            FileEvent        `field:"-" json:"-"`
	EventKind       SELinuxEventKind `field:"-" json:"-"`
	BoolName        string           `field:"bool.name,handler:ResolveSELinuxBoolName"` // SECLDoc[bool.name] Definition:`SELinux boolean name`
	BoolChangeValue string           `field:"bool.state"`                               // SECLDoc[bool.state] Definition:`SELinux boolean new value`
	BoolCommitValue bool             `field:"bool_commit.state"`                        // SECLDoc[bool_commit.state] Definition:`Indicator of a SELinux boolean commit operation`
	EnforceStatus   string           `field:"enforce.status"`                           // SECLDoc[enforce.status] Definition:`SELinux enforcement status (one of "enforcing", "permissive", "disabled")`
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

// PIDContext holds the process context of an kernel event
type PIDContext struct {
	Pid       uint32 `field:"pid"` // SECLDoc[pid] Definition:`Process ID of the process (also called thread group ID)`
	Tid       uint32 `field:"tid"` // SECLDoc[tid] Definition:`Thread ID of the thread`
	NetNS     uint32 `field:"-"`
	IsKworker bool   `field:"is_kworker"` // SECLDoc[is_kworker] Definition:`Indicates whether the process is a kworker`
	Inode     uint64 `field:"-"`          // used to track exec and event loss
}

// RenameEvent represents a rename event
type RenameEvent struct {
	SyscallEvent
	Old FileEvent `field:"file"`
	New FileEvent `field:"file.destination"`
}

// RmdirEvent represents a rmdir event
type RmdirEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
}

// SetXAttrEvent represents an extended attributes event
type SetXAttrEvent struct {
	SyscallEvent
	File      FileEvent `field:"file"`
	Namespace string    `field:"file.destination.namespace,handler:ResolveXAttrNamespace"` // SECLDoc[file.destination.namespace] Definition:`Namespace of the extended attribute`
	Name      string    `field:"file.destination.name,handler:ResolveXAttrName"`           // SECLDoc[file.destination.name] Definition:`Name of the extended attribute`

	NameRaw [200]byte `field:"-" json:"-"`
}

// SyscallEvent contains common fields for all the event
type SyscallEvent struct {
	Retval int64 `field:"retval"` // SECLDoc[retval] Definition:`Return value of the syscall` Constants:`Error constants`
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	Flags uint32    `field:"flags"` // SECLDoc[flags] Definition:`Flags of the unlink syscall` Constants:`Unlink flags`
}

// UmountEvent represents an umount event
type UmountEvent struct {
	SyscallEvent
	MountID uint32
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	Atime time.Time `field:"-" json:"-"`
	Mtime time.Time `field:"-" json:"-"`
}

// BPFEvent represents a BPF event
type BPFEvent struct {
	SyscallEvent

	Map     BPFMap     `field:"map"`  // eBPF map involved in the BPF command
	Program BPFProgram `field:"prog"` // eBPF program involved in the BPF command
	Cmd     uint32     `field:"cmd"`  // SECLDoc[cmd] Definition:`BPF command name` Constants:`BPF commands`
}

// BPFMap represents a BPF map
type BPFMap struct {
	ID   uint32 `field:"-" json:"-"` // ID of the eBPF map
	Type uint32 `field:"type"`       // SECLDoc[type] Definition:`Type of the eBPF map` Constants:`BPF map types`
	Name string `field:"name"`       // SECLDoc[name] Definition:`Name of the eBPF map (added in 7.35)`
}

// BPFProgram represents a BPF program
type BPFProgram struct {
	ID         uint32   `field:"-" json:"-"`  // ID of the eBPF program
	Type       uint32   `field:"type"`        // SECLDoc[type] Definition:`Type of the eBPF program` Constants:`BPF program types`
	AttachType uint32   `field:"attach_type"` // SECLDoc[attach_type] Definition:`Attach type of the eBPF program` Constants:`BPF attach types`
	Helpers    []uint32 `field:"helpers"`     // SECLDoc[helpers] Definition:`eBPF helpers used by the eBPF program (added in 7.35)` Constants:`BPF helper functions`
	Name       string   `field:"name"`        // SECLDoc[name] Definition:`Name of the eBPF program (added in 7.35)`
	Tag        string   `field:"tag"`         // SECLDoc[tag] Definition:`Hash (sha1) of the eBPF program (added in 7.35)`
}

// PTraceEvent represents a ptrace event
type PTraceEvent struct {
	SyscallEvent

	Request uint32          `field:"request"` // SECLDoc[request] Definition:`ptrace request` Constants:`Ptrace constants`
	PID     uint32          `field:"-" json:"-"`
	Address uint64          `field:"-" json:"-"`
	Tracee  *ProcessContext `field:"tracee"` // process context of the tracee
}

// MMapEvent represents a mmap event
type MMapEvent struct {
	SyscallEvent

	File       FileEvent `field:"file"`
	Addr       uint64    `field:"-" json:"-"`
	Offset     uint64    `field:"-" json:"-"`
	Len        uint32    `field:"-" json:"-"`
	Protection int       `field:"protection"` // SECLDoc[protection] Definition:`memory segment protection` Constants:`Protection constants`
	Flags      int       `field:"flags"`      // SECLDoc[flags] Definition:`memory segment flags` Constants:`MMap flags`
}

// MProtectEvent represents a mprotect event
type MProtectEvent struct {
	SyscallEvent

	VMStart       uint64 `field:"-" json:"-"`
	VMEnd         uint64 `field:"-" json:"-"`
	VMProtection  int    `field:"vm_protection"`  // SECLDoc[vm_protection] Definition:`initial memory segment protection` Constants:`Virtual Memory flags`
	ReqProtection int    `field:"req_protection"` // SECLDoc[req_protection] Definition:`new memory segment protection` Constants:`Virtual Memory flags`
}

// LoadModuleEvent represents a load_module event
type LoadModuleEvent struct {
	SyscallEvent

	File             FileEvent `field:"file"`                           // Path to the kernel module file
	LoadedFromMemory bool      `field:"loaded_from_memory"`             // SECLDoc[loaded_from_memory] Definition:`Indicates if the kernel module was loaded from memory`
	Name             string    `field:"name"`                           // SECLDoc[name] Definition:`Name of the new kernel module`
	Args             string    `field:"args,handler:ResolveModuleArgs"` // SECLDoc[args] Definition:`Parameters (as a string) of the new kernel module`
	Argv             []string  `field:"argv,handler:ResolveModuleArgv"` // SECLDoc[argv] Definition:`Parameters (as an array) of the new kernel module`
	ArgsTruncated    bool      `field:"args_truncated"`                 // SECLDoc[args_truncated] Definition:`Indicates if the arguments were truncated or not`
}

// UnloadModuleEvent represents an unload_module event
type UnloadModuleEvent struct {
	SyscallEvent

	Name string `field:"name"` // SECLDoc[name] Definition:`Name of the kernel module that was deleted`
}

// SignalEvent represents a signal event
type SignalEvent struct {
	SyscallEvent

	Type   uint32          `field:"type"`   // SECLDoc[type] Definition:`Signal type (ex: SIGHUP, SIGINT, SIGQUIT, etc)` Constants:`Signal constants`
	PID    uint32          `field:"pid"`    // SECLDoc[pid] Definition:`Target PID`
	Target *ProcessContext `field:"target"` // Target process context
}

// SpliceEvent represents a splice event
type SpliceEvent struct {
	SyscallEvent

	File          FileEvent `field:"file"`            // File modified by the splice syscall
	PipeEntryFlag uint32    `field:"pipe_entry_flag"` // SECLDoc[pipe_entry_flag] Definition:`Entry flag of the "fd_out" pipe passed to the splice syscall` Constants:`Pipe buffer flags`
	PipeExitFlag  uint32    `field:"pipe_exit_flag"`  // SECLDoc[pipe_exit_flag] Definition:`Exit flag of the "fd_out" pipe passed to the splice syscall` Constants:`Pipe buffer flags`
}

// CgroupTracingEvent is used to signal that a new cgroup should be traced by the activity dump manager
type CgroupTracingEvent struct {
	ContainerContext ContainerContext
	Config           ActivityDumpLoadConfig
	ConfigCookie     uint32
}

// ActivityDumpLoadConfig represents the load configuration of an activity dump
type ActivityDumpLoadConfig struct {
	TracedEventTypes     []EventType
	Timeout              time.Duration
	WaitListTimestampRaw uint64
	StartTimestampRaw    uint64
	EndTimestampRaw      uint64
	Rate                 uint32 // max number of events per sec
	Paused               uint32
}

// SetTimeout updates the timeout of an activity dump
func (adlc *ActivityDumpLoadConfig) SetTimeout(duration time.Duration) {
	adlc.Timeout = duration
	adlc.EndTimestampRaw = adlc.StartTimestampRaw + uint64(duration)
}

// NetworkDeviceContext represents the network device context of a network event
type NetworkDeviceContext struct {
	NetNS   uint32 `field:"-" json:"-"`
	IfIndex uint32 `field:"ifindex"`                                   // SECLDoc[ifindex] Definition:`interface ifindex`
	IfName  string `field:"ifname,handler:ResolveNetworkDeviceIfName"` // SECLDoc[ifname] Definition:`interface ifname`
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

// DNSEvent represents a DNS event
type DNSEvent struct {
	ID    uint16 `field:"id" json:"-"`                                             // SECLDoc[id] Definition:`[Experimental] the DNS request ID`
	Name  string `field:"question.name,opts:length" op_override:"eval.DNSNameCmp"` // SECLDoc[question.name] Definition:`the queried domain name`
	Type  uint16 `field:"question.type"`                                           // SECLDoc[question.type] Definition:`a two octet code which specifies the DNS question type` Constants:`DNS qtypes`
	Class uint16 `field:"question.class"`                                          // SECLDoc[question.class] Definition:`the class looked up by the DNS question` Constants:`DNS qclasses`
	Size  uint16 `field:"question.length"`                                         // SECLDoc[question.length] Definition:`the total DNS request size in bytes`
	Count uint16 `field:"question.count"`                                          // SECLDoc[question.count] Definition:`the total count of questions in the DNS request`
}

// BindEvent represents a bind event
type BindEvent struct {
	SyscallEvent

	Addr       IPPortContext `field:"addr"`        // Bound address
	AddrFamily uint16        `field:"addr.family"` // SECLDoc[addr.family] Definition:`Address family`
}

// NetDevice represents a network device
type NetDevice struct {
	Name        string
	NetNS       uint32
	IfIndex     uint32
	PeerNetNS   uint32
	PeerIfIndex uint32
}

// GetKey returns a key to uniquely identify a network device on the system
func (d NetDevice) GetKey() string {
	return fmt.Sprintf("%v_%v", d.IfIndex, d.NetNS)
}

// NetDeviceEvent represents a network device event
type NetDeviceEvent struct {
	SyscallEvent

	Device NetDevice
}

// VethPairEvent represents a veth pair event
type VethPairEvent struct {
	SyscallEvent

	HostDevice NetDevice
	PeerDevice NetDevice
}

// SyscallsEvent represents a syscalls event
type SyscallsEvent struct {
	Syscalls []Syscall // 64 * 8 = 512 > 450, bytes should be enough to hold all 450 syscalls
}

const PathKeySize = 16

// AnomalyDetectionSyscallEvent represents an anomaly detection for a syscall event
type AnomalyDetectionSyscallEvent struct {
	SyscallID Syscall
}

// PathKey identifies an entry in the dentry cache
type PathKey struct {
	Inode   uint64 `field:"inode"`    // SECLDoc[inode] Definition:`Inode of the file`
	MountID uint32 `field:"mount_id"` // SECLDoc[mount_id] Definition:`Mount ID of the file`
	PathID  uint32 `field:"-"`
}

func (p *PathKey) Write(buffer []byte) {
	ByteOrder.PutUint64(buffer[0:8], p.Inode)
	ByteOrder.PutUint32(buffer[8:12], p.MountID)
	ByteOrder.PutUint32(buffer[12:16], p.PathID)
}

// IsNull returns true if a key is invalid
func (p *PathKey) IsNull() bool {
	return p.Inode == 0 && p.MountID == 0
}

func (p *PathKey) String() string {
	return fmt.Sprintf("%x/%x", p.MountID, p.Inode)
}

// MarshalBinary returns the binary representation of a path key
func (p *PathKey) MarshalBinary() ([]byte, error) {
	if p.IsNull() {
		return nil, &ErrInvalidKeyPath{Inode: p.Inode, MountID: p.MountID}
	}

	buff := make([]byte, 16)
	p.Write(buff)

	return buff, nil
}

// PathLeafSize defines path_leaf struct size
const PathLeafSize = PathKeySize + MaxSegmentLength + 1 + 2 + 6 // path_key + name + len + padding

// PathLeaf is the go representation of the eBPF path_leaf_t structure
type PathLeaf struct {
	Parent PathKey
	Name   [MaxSegmentLength + 1]byte
	Len    uint16
}

// GetName returns the path value as a string
func (pl *PathLeaf) GetName() string {
	return NullTerminatedString(pl.Name[:])
}

// GetName returns the path value as a string
func (pl *PathLeaf) SetName(name string) {
	copy(pl.Name[:], []byte(name))
	pl.Len = uint16(len(name) + 1)
}

// MarshalBinary returns the binary representation of a path key
func (pl *PathLeaf) MarshalBinary() ([]byte, error) {
	buff := make([]byte, PathLeafSize)

	pl.Parent.Write(buff)
	copy(buff[16:], pl.Name[:])
	ByteOrder.PutUint16(buff[16+len(pl.Name):], pl.Len)

	return buff, nil
}

// ExtraFieldHandlers handlers not hold by any field
type ExtraFieldHandlers interface {
	ResolveProcessCacheEntry(ev *Event) (*ProcessCacheEntry, bool)
	ResolveEventTimestamp(ev *Event) time.Time
	GetProcessServiceTag(ev *Event) string
}

// ResolveProcessCacheEntry stub implementation
func (dfh *DefaultFieldHandlers) ResolveProcessCacheEntry(ev *Event) (*ProcessCacheEntry, bool) {
	return nil, false
}

// ResolveEventTimestamp stub implementation
func (dfh *DefaultFieldHandlers) ResolveEventTimestamp(ev *Event) time.Time {
	return ev.Timestamp
}

// GetProcessServiceTag stub implementation
func (dfh *DefaultFieldHandlers) GetProcessServiceTag(ev *Event) string {
	return ""
}
