// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors -mock -output accessors.go
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors -tags linux -output ../../probe/accessors.go -doc ../../../../docs/cloud-workload-security/secl.json -fields-resolver ../../probe/fields_resolver.go

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
type Model struct{}

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{}
}

// NewEventWithType returns a new Event for the given type
func (m *Model) NewEventWithType(kind EventType) eval.Event {
	return &Event{
		Type: uint32(kind),
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

	return nil
}

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode; file.destination.rights" constants:"Chmod mode constants"` // New mode/rights of the chmod-ed file
}

// ChownEvent represents a chown event
type ChownEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	UID   int64     `field:"file.destination.uid"`                           // New UID of the chown-ed file's owner
	User  string    `field:"file.destination.user,handler:ResolveChownUID"`  // New user of the chown-ed file's owner
	GID   int64     `field:"file.destination.gid"`                           // New GID of the chown-ed file's owner
	Group string    `field:"file.destination.group,handler:ResolveChownGID"` // New group of the chown-ed file's owner
}

// ContainerContext holds the container context of an event
type ContainerContext struct {
	ID   string   `field:"id,handler:ResolveContainerID"`                              // ID of the container
	Tags []string `field:"tags,handler:ResolveContainerTags,opts:skip_ad,weight:9999"` // Tags of the container
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	ID                   string    `field:"-" json:"-"`
	Type                 uint32    `field:"-"`
	Async                bool      `field:"async" event:"*"` // True if the syscall was asynchronous
	SavedByActivityDumps bool      `field:"-"`               // True if the event should have been discarded if the AD were disabled
	IsActivityDumpSample bool      `field:"-"`               // True if the event was sampled for the activity dumps
	TimestampRaw         uint64    `field:"-" json:"-"`
	Timestamp            time.Time `field:"-"` // Timestamp of the event

	// context shared with all events
	ProcessCacheEntry *ProcessCacheEntry `field:"-" json:"-"`
	PIDContext        PIDContext         `field:"-" json:"-"`
	SpanContext       SpanContext        `field:"-" json:"-"`
	ProcessContext    *ProcessContext    `field:"process" event:"*"`
	ContainerContext  ContainerContext   `field:"container"`
	NetworkContext    NetworkContext     `field:"network"`

	// fim events
	Chmod       ChmodEvent    `field:"chmod" event:"chmod"`             // [7.27] [File] A file’s permissions were changed
	Chown       ChownEvent    `field:"chown" event:"chown"`             // [7.27] [File] A file’s owner was changed
	Open        OpenEvent     `field:"open" event:"open"`               // [7.27] [File] A file was opened
	Mkdir       MkdirEvent    `field:"mkdir" event:"mkdir"`             // [7.27] [File] A directory was created
	Rmdir       RmdirEvent    `field:"rmdir" event:"rmdir"`             // [7.27] [File] A directory was removed
	Rename      RenameEvent   `field:"rename" event:"rename"`           // [7.27] [File] A file/directory was renamed
	Unlink      UnlinkEvent   `field:"unlink" event:"unlink"`           // [7.27] [File] A file was deleted
	Utimes      UtimesEvent   `field:"utimes" event:"utimes"`           // [7.27] [File] Change file access/modification times
	Link        LinkEvent     `field:"link" event:"link"`               // [7.27] [File] Create a new name/alias for a file
	SetXAttr    SetXAttrEvent `field:"setxattr" event:"setxattr"`       // [7.27] [File] Set exteneded attributes
	RemoveXAttr SetXAttrEvent `field:"removexattr" event:"removexattr"` // [7.27] [File] Remove extended attributes
	Splice      SpliceEvent   `field:"splice" event:"splice"`           // [7.36] [File] A splice command was executed

	// process events
	Exec     ExecEvent     `field:"exec" event:"exec"`     // [7.27] [Process] A process was executed or forked
	SetUID   SetuidEvent   `field:"setuid" event:"setuid"` // [7.27] [Process] A process changed its effective uid
	SetGID   SetgidEvent   `field:"setgid" event:"setgid"` // [7.27] [Process] A process changed its effective gid
	Capset   CapsetEvent   `field:"capset" event:"capset"` // [7.27] [Process] A process changed its capacity set
	Signal   SignalEvent   `field:"signal" event:"signal"` // [7.35] [Process] A signal was sent
	Exit     ExitEvent     `field:"exit" event:"exit"`     // [7.38] [Process] A process was terminated
	Syscalls SyscallsEvent `field:"-"`

	// kernel events
	SELinux      SELinuxEvent      `field:"selinux" event:"selinux"`             // [7.30] [Kernel] An SELinux operation was run
	BPF          BPFEvent          `field:"bpf" event:"bpf"`                     // [7.33] [Kernel] A BPF command was executed
	PTrace       PTraceEvent       `field:"ptrace" event:"ptrace"`               // [7.35] [Kernel] A ptrace command was executed
	MMap         MMapEvent         `field:"mmap" event:"mmap"`                   // [7.35] [Kernel] A mmap command was executed
	MProtect     MProtectEvent     `field:"mprotect" event:"mprotect"`           // [7.35] [Kernel] A mprotect command was executed
	LoadModule   LoadModuleEvent   `field:"load_module" event:"load_module"`     // [7.35] [Kernel] A new kernel module was loaded
	UnloadModule UnloadModuleEvent `field:"unload_module" event:"unload_module"` // [7.35] [Kernel] A kernel module was deleted

	// network events
	DNS  DNSEvent  `field:"dns" event:"dns"`   // [7.36] [Network] A DNS request was sent
	Bind BindEvent `field:"bind" event:"bind"` // [7.37] [Network] [Experimental] A bind was executed

	// internal usage
	Mount            MountEvent            `field:"-" json:"-"`
	Umount           UmountEvent           `field:"-" json:"-"`
	InvalidateDentry InvalidateDentryEvent `field:"-" json:"-"`
	ArgsEnvs         ArgsEnvsEvent         `field:"-" json:"-"`
	MountReleased    MountReleasedEvent    `field:"-" json:"-"`
	CgroupTracing    CgroupTracingEvent    `field:"-" json:"-"`
	NetDevice        NetDeviceEvent        `field:"-" json:"-"`
	VethPair         VethPairEvent         `field:"-" json:"-"`
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

// GetPointer return an unsafe.Pointer of the Event
func (e *Event) GetPointer() unsafe.Pointer {
	return unsafe.Pointer(e)
}

// SetuidEvent represents a setuid event
type SetuidEvent struct {
	UID    uint32 `field:"uid"`                                // New UID of the process
	User   string `field:"user,handler:ResolveSetuidUser"`     // New user of the process
	EUID   uint32 `field:"euid"`                               // New effective UID of the process
	EUser  string `field:"euser,handler:ResolveSetuidEUser"`   // New effective user of the process
	FSUID  uint32 `field:"fsuid"`                              // New FileSystem UID of the process
	FSUser string `field:"fsuser,handler:ResolveSetuidFSUser"` // New FileSystem user of the process
}

// SetgidEvent represents a setgid event
type SetgidEvent struct {
	GID     uint32 `field:"gid"`                                  // New GID of the process
	Group   string `field:"group,handler:ResolveSetgidGroup"`     // New group of the process
	EGID    uint32 `field:"egid"`                                 // New effective GID of the process
	EGroup  string `field:"egroup,handler:ResolveSetgidEGroup"`   // New effective group of the process
	FSGID   uint32 `field:"fsgid"`                                // New FileSystem GID of the process
	FSGroup string `field:"fsgroup,handler:ResolveSetgidFSGroup"` // New FileSystem group of the process
}

// CapsetEvent represents a capset event
type CapsetEvent struct {
	CapEffective uint64 `field:"cap_effective" constants:"Kernel Capability constants"` // Effective capability set of the process
	CapPermitted uint64 `field:"cap_permitted" constants:"Kernel Capability constants"` // Permitted capability set of the process
}

// Credentials represents the kernel credentials of a process
type Credentials struct {
	UID   uint32 `field:"uid"`   // UID of the process
	GID   uint32 `field:"gid"`   // GID of the process
	User  string `field:"user"`  // User of the process
	Group string `field:"group"` // Group of the process

	EUID   uint32 `field:"euid"`   // Effective UID of the process
	EGID   uint32 `field:"egid"`   // Effective GID of the process
	EUser  string `field:"euser"`  // Effective user of the process
	EGroup string `field:"egroup"` // Effective group of the process

	FSUID   uint32 `field:"fsuid"`   // FileSystem-uid of the process
	FSGID   uint32 `field:"fsgid"`   // FileSystem-gid of the process
	FSUser  string `field:"fsuser"`  // FileSystem-user of the process
	FSGroup string `field:"fsgroup"` // FileSystem-group of the process

	CapEffective uint64 `field:"cap_effective" constants:"Kernel Capability constants"` // Effective capability set of the process
	CapPermitted uint64 `field:"cap_permitted" constants:"Kernel Capability constants"` // Permitted capability set of the process
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (p *Process) GetPathResolutionError() string {
	if p.FileEvent.PathResolutionError != nil {
		return p.FileEvent.PathResolutionError.Error()
	}
	return ""
}

// HasInterpreter returns whether the process uses an interpreter
func (p *Process) HasInterpreter() bool {
	return p.LinuxBinprm.FileEvent.Inode != 0 && p.LinuxBinprm.FileEvent.MountID != 0
}

// LinuxBinprm contains content from the linux_binprm struct, which holds the arguments used for loading binaries
type LinuxBinprm struct {
	FileEvent FileEvent `field:"file"`
}

// Process represents a process
type Process struct {
	PIDContext

	FileEvent FileEvent `field:"file"`

	ContainerID   string   `field:"container.id"` // Container ID
	ContainerTags []string `field:"-"`

	SpanID  uint64 `field:"-"`
	TraceID uint64 `field:"-"`

	TTYName     string      `field:"tty_name"`    // Name of the TTY associated with the process
	Comm        string      `field:"comm"`        // Comm attribute of the process
	LinuxBinprm LinuxBinprm `field:"interpreter"` // Script interpreter as identified by the shebang

	// pid_cache_t
	ForkTime time.Time `field:"-" json:"-"`
	ExitTime time.Time `field:"-" json:"-"`
	ExecTime time.Time `field:"-" json:"-"`

	CreatedAt uint64 `field:"created_at,handler:ResolveProcessCreatedAt"` // Timestamp of the creation of the process

	Cookie uint32 `field:"cookie"` // Cookie of the process
	PPid   uint32 `field:"ppid"`   // Parent process ID

	// credentials_t section of pid_cache_t
	Credentials ``

	ArgsID uint32 `field:"-" json:"-"`
	EnvsID uint32 `field:"-" json:"-"`

	ArgsEntry *ArgsEntry `field:"-" json:"-"`
	EnvsEntry *EnvsEntry `field:"-" json:"-"`

	// defined to generate accessors, ArgsTruncated and EnvsTruncated are used during by unmarshaller
	Argv0         string   `field:"argv0,handler:ResolveProcessArgv0,weight:100"`                                                                                                                                               // First argument of the process
	Args          string   `field:"args,handler:ResolveProcessArgs,weight:100"`                                                                                                                                                 // Arguments of the process (as a string)
	Argv          []string `field:"argv,handler:ResolveProcessArgv,weight:100; args_flags,handler:ResolveProcessArgsFlags,opts:cacheless_resolution; args_options,handler:ResolveProcessArgsOptions,opts:cacheless_resolution"` // Arguments of the process (as an array)
	ArgsTruncated bool     `field:"args_truncated,handler:ResolveProcessArgsTruncated"`                                                                                                                                         // Indicator of arguments truncation
	Envs          []string `field:"envs,handler:ResolveProcessEnvs:100"`                                                                                                                                                        // Environment variable names of the process
	Envp          []string `field:"envp,handler:ResolveProcessEnvp:100"`                                                                                                                                                        // Environment variables of the process
	EnvsTruncated bool     `field:"envs_truncated,handler:ResolveProcessEnvsTruncated"`                                                                                                                                         // Indicator of environment variables truncation

	// symlink to the process binary
	SymlinkPathnameStr [MaxSymlinks]string `field:"-" json:"-"`
	SymlinkBasenameStr string              `field:"-" json:"-"`

	// cache version
	ScrubbedArgvResolved  bool           `field:"-" json:"-"`
	ScrubbedArgv          []string       `field:"-" json:"-"`
	ScrubbedArgsTruncated bool           `field:"-" json:"-"`
	Variables             eval.Variables `field:"-" json:"-"`

	IsThread bool `field:"is_thread"` // Indicates whether the process is considered a thread (that is, a child process that hasn't executed another program)
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
	Cause uint32 `field:"cause"` // Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)
	Code  uint32 `field:"code"`  // Exit code of the process or number of the signal that caused the process to terminate
}

// FileFields holds the information required to identify a file
type FileFields struct {
	UID   uint32 `field:"uid"`                                                                                          // UID of the file's owner
	User  string `field:"user,handler:ResolveFileFieldsUser"`                                                           // User of the file's owner
	GID   uint32 `field:"gid"`                                                                                          // GID of the file's owner
	Group string `field:"group,handler:ResolveFileFieldsGroup"`                                                         // Group of the file's owner
	Mode  uint16 `field:"mode;rights,handler:ResolveRights,opts:cacheless_resolution" constants:"Chmod mode constants"` // Mode/rights of the file
	CTime uint64 `field:"change_time"`                                                                                  // Change time of the file
	MTime uint64 `field:"modification_time"`                                                                            // Modification time of the file

	MountID      uint32 `field:"mount_id"`                                             // Mount ID of the file
	Inode        uint64 `field:"inode"`                                                // Inode of the file
	InUpperLayer bool   `field:"in_upper_layer,handler:ResolveFileFieldsInUpperLayer"` // Indicator of the file layer, for example, in an OverlayFS

	NLink  uint32 `field:"-" json:"-"`
	PathID uint32 `field:"-" json:"-"`
	Flags  int32  `field:"-" json:"-"`
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

	PathnameStr string `field:"path,handler:ResolveFilePath,opts:length" op_override:"ProcessSymlinkPathname"`     // File's path
	BasenameStr string `field:"name,handler:ResolveFileBasename,opts:length" op_override:"ProcessSymlinkBasename"` // File's basename
	Filesystem  string `field:"filesystem,handler:ResolveFileFilesystem"`                                          // File's filesystem

	PathResolutionError error `field:"-" json:"-"`

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
	Mode uint32    `field:"file.destination.mode; file.destination.rights" constants:"Chmod mode constants"` // Mode/rights of the new directory
}

// ArgsEnvsEvent defines a args/envs event
type ArgsEnvsEvent struct {
	ArgsEnvs
}

// MountEvent represents a mount event
type MountEvent struct {
	SyscallEvent
	MountID       uint32
	GroupID       uint32
	Device        uint32
	ParentMountID uint32
	ParentInode   uint64
	FSType        string
	MountPointStr string
	RootMountID   uint32
	RootInode     uint64
	RootStr       string

	FSTypeRaw [16]byte
}

// GetFSType returns the filesystem type of the mountpoint
func (m *MountEvent) GetFSType() string {
	return m.FSType
}

// IsOverlayFS returns whether it is an overlay fs
func (m *MountEvent) IsOverlayFS() bool {
	return m.GetFSType() == OverlayFS
}

// OpenEvent represents an open event
type OpenEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	Flags uint32    `field:"flags" constants:"Open flags"`                           // Flags used when opening the file
	Mode  uint32    `field:"file.destination.mode" constants:"Chmod mode constants"` // Mode of the created file
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
	BoolName        string           `field:"bool.name,handler:ResolveSELinuxBoolName"` // SELinux boolean name
	BoolChangeValue string           `field:"bool.state"`                               // SELinux boolean new value
	BoolCommitValue bool             `field:"bool_commit.state"`                        // Indicator of a SELinux boolean commit operation
	EnforceStatus   string           `field:"enforce.status"`                           // SELinux enforcement status (one of "enforcing", "permissive", "disabled"")
}

var zeroProcessContext ProcessContext

// ProcessCacheEntry this struct holds process context kept in the process tree
type ProcessCacheEntry struct {
	ProcessContext

	refCount  uint64                     `field:"-" json:"-"`
	onRelease func(_ *ProcessCacheEntry) `field:"-" json:"-"`
	releaseCb func()                     `field:"-" json:"-"`
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
	pc.releaseCb = callback
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
	if front := (*Event)(ctx.Object).ProcessContext.Ancestor; front != nil {
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

// ProcessContext holds the process context of an event
type ProcessContext struct {
	Process

	Ancestor *ProcessCacheEntry `field:"ancestors,iterator:ProcessAncestorsIterator"`
}

// PIDContext holds the process context of an kernel event
type PIDContext struct {
	Pid       uint32 `field:"pid"` // Process ID of the process (also called thread group ID)
	Tid       uint32 `field:"tid"` // Thread ID of the thread
	NetNS     uint32 `field:"-"`
	IsKworker bool   `field:"is_kworker"` // Indicates whether the process is a kworker
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
	Namespace string    `field:"file.destination.namespace,handler:ResolveXAttrNamespace"` // Namespace of the extended attribute
	Name      string    `field:"file.destination.name,handler:ResolveXAttrName"`           // Name of the extended attribute

	NameRaw [200]byte `field:"-" json:"-"`
}

// SyscallEvent contains common fields for all the event
type SyscallEvent struct {
	Retval int64 `field:"retval" constants:"Error Constants"` // Return value of the syscall
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	Flags uint32    `field:"flags" constants:"Unlink flags"`
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

	Map     BPFMap     `field:"map"`                          // eBPF map involved in the BPF command
	Program BPFProgram `field:"prog"`                         // eBPF program involved in the BPF command
	Cmd     uint32     `field:"cmd" constants:"BPF commands"` // BPF command name
}

// BPFMap represents a BPF map
type BPFMap struct {
	ID   uint32 `field:"-" json:"-"`                     // ID of the eBPF map
	Type uint32 `field:"type" constants:"BPF map types"` // Type of the eBPF map
	Name string `field:"name"`                           // Name of the eBPF map (added in 7.35)
}

// BPFProgram represents a BPF program
type BPFProgram struct {
	ID         uint32   `field:"-" json:"-"`                                                      // ID of the eBPF program
	Type       uint32   `field:"type" constants:"BPF program types"`                              // Type of the eBPF program
	AttachType uint32   `field:"attach_type" constants:"BPF attach types"`                        // Attach type of the eBPF program
	Helpers    []uint32 `field:"helpers,handler:ResolveHelpers" constants:"BPF helper functions"` // eBPF helpers used by the eBPF program (added in 7.35)
	Name       string   `field:"name"`                                                            // Name of the eBPF program (added in 7.35)
	Tag        string   `field:"tag"`                                                             // Hash (sha1) of the eBPF program (added in 7.35)
}

// PTraceEvent represents a ptrace event
type PTraceEvent struct {
	SyscallEvent

	Request uint32          `field:"request" constants:"Ptrace constants"` //  ptrace request
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
	Protection int       `field:"protection" constants:"Protection constants"` // memory segment protection
	Flags      int       `field:"flags" constants:"MMap flags"`                // memory segment flags
}

// MProtectEvent represents a mprotect event
type MProtectEvent struct {
	SyscallEvent

	VMStart       uint64 `field:"-" json:"-"`
	VMEnd         uint64 `field:"-" json:"-"`
	VMProtection  int    `field:"vm_protection" constants:"Virtual Memory flags"`  // initial memory segment protection
	ReqProtection int    `field:"req_protection" constants:"Virtual Memory flags"` // new memory segment protection
}

// LoadModuleEvent represents a load_module event
type LoadModuleEvent struct {
	SyscallEvent

	File             FileEvent `field:"file"`               // Path to the kernel module file
	LoadedFromMemory bool      `field:"loaded_from_memory"` // Indicates if the kernel module was loaded from memory
	Name             string    `field:"name"`               // Name of the new kernel module
}

// UnloadModuleEvent represents an unload_module event
type UnloadModuleEvent struct {
	SyscallEvent

	Name string `field:"name"` // Name of the kernel module that was deleted
}

// SignalEvent represents a signal event
type SignalEvent struct {
	SyscallEvent

	Type   uint32          `field:"type" constants:"Signal constants"` // Signal type (ex: SIGHUP, SIGINT, SIGQUIT, etc)
	PID    uint32          `field:"pid"`                               // Target PID
	Target *ProcessContext `field:"target"`                            // Target process context
}

// SpliceEvent represents a splice event
type SpliceEvent struct {
	SyscallEvent

	File          FileEvent `field:"file"`                                          // File modified by the splice syscall
	PipeEntryFlag uint32    `field:"pipe_entry_flag" constants:"Pipe buffer flags"` // Entry flag of the "fd_out" pipe passed to the splice syscall
	PipeExitFlag  uint32    `field:"pipe_exit_flag" constants:"Pipe buffer flags"`  // Exit flag of the "fd_out" pipe passed to the splice syscall
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
	IfIndex uint32 `field:"ifindex"`                                   // interface ifindex
	IfName  string `field:"ifname,handler:ResolveNetworkDeviceIfName"` // interface ifname
}

// IPPortContext is used to hold an IP and Port
type IPPortContext struct {
	IPNet net.IPNet `field:"ip"`   // IP address
	Port  uint16    `field:"port"` // Port number
}

// NetworkContext represents the network context of the event
type NetworkContext struct {
	Device NetworkDeviceContext `field:"device"` // network device on which the network packet was captured

	L3Protocol  uint16        `field:"l3_protocol" constants:"L3 protocols"` // l3 protocol of the network packet
	L4Protocol  uint16        `field:"l4_protocol" constants:"L4 protocols"` // l4 protocol of the network packet
	Source      IPPortContext `field:"source"`                               // source of the network packet
	Destination IPPortContext `field:"destination"`                          // destination of the network packet
	Size        uint32        `field:"size"`                                 // size in bytes of the network packet
}

// DNSEvent represents a DNS event
type DNSEvent struct {
	ID    uint16 `field:"id" json:"-"`                                             // [Experimental] the DNS request ID
	Name  string `field:"question.name,opts:length" op_override:"eval.DNSNameCmp"` // the queried domain name
	Type  uint16 `field:"question.type" constants:"DNS qtypes"`                    // a two octet code which specifies the DNS question type
	Class uint16 `field:"question.class" constants:"DNS qclasses"`                 // the class looked up by the DNS question
	Size  uint16 `field:"question.length"`                                         // the total DNS request size in bytes
	Count uint16 `field:"question.count"`                                          // the total count of questions in the DNS request
}

// BindEvent represents a bind event
type BindEvent struct {
	SyscallEvent

	Addr       IPPortContext `field:"addr"`        // Bound address
	AddrFamily uint16        `field:"addr.family"` // Address family
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
