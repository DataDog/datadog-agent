// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package model

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

const (
	OverlayFS = "overlay" // OverlayFS overlay filesystem
	TmpFS     = "tmpfs"   // TmpFS tmpfs
	UnknownFS = "unknown" // UnknownFS unknown filesystem

	ErrPathMustBeAbsolute = "all the path have to be absolute"            // ErrPathMustBeAbsolute tells when a path is not absolute
	ErrPathDepthLimit     = "path depths have to be shorter than"         // ErrPathDepthLimit tells when a path is too long
	ErrPathSegmentLimit   = "each segment of a path must be shorter than" // ErrPathSegmentLimit tells when a patch reached the segment limit

	// SizeOfCookie size of cookie
	SizeOfCookie = 8

	// FakeInodeMSW inode used internally
	fakeInodeMSW uint64 = 0xdeadc001
)

// check that all path are absolute
func validatePath(field eval.Field, fieldValue eval.FieldValue) error {
	// do not support regular expression on path, currently unable to support discarder for regex value
	if fieldValue.Type == eval.RegexpValueType {
		return fmt.Errorf("regexp not supported on path `%s`", field)
	} else if fieldValue.Type == eval.VariableValueType {
		return nil
	}

	if value, ok := fieldValue.Value.(string); ok {
		errAbs := fmt.Errorf("invalid path `%s`, %s", value, ErrPathMustBeAbsolute)
		errDepth := fmt.Errorf("invalid path `%s`, %s %d", value, ErrPathDepthLimit, MaxPathDepth)
		errSegment := fmt.Errorf("invalid path `%s`, %s %d", value, ErrPathSegmentLimit, MaxSegmentLength)

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
	if strings.HasSuffix(field, ".path") && !strings.HasSuffix(field, ".syscall.path") {
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

// IsFakeInode returns whether the given inode is a fake inode
func IsFakeInode(inode uint64) bool {
	return inode>>32 == fakeInodeMSW
}

// SetPathResolutionError sets the Event.pathResolutionError
func (ev *Event) SetPathResolutionError(fileFields *FileEvent, err error) {
	fileFields.PathResolutionError = err
	ev.Error = err
}

// Equals returns if both credentials are equal
func (c *Credentials) Equals(o *Credentials) bool {
	return c.UID == o.UID &&
		c.GID == o.GID &&
		c.EUID == o.EUID &&
		c.EGID == o.EGID &&
		c.FSUID == o.FSUID &&
		c.FSGID == o.FSGID &&
		c.CapEffective == o.CapEffective &&
		c.CapPermitted == o.CapPermitted
}

// SetSpan sets the span
func (p *Process) SetSpan(spanID uint64, traceID utils.TraceID) {
	p.SpanID = spanID
	p.TraceID = traceID
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

// GetProcessArgv returns the unscrubbed args of the event as an array. Use with caution.
func (p *Process) GetProcessArgv() ([]string, bool) {
	if p.ArgsEntry == nil {
		return p.Argv, p.ArgsTruncated
	}

	argv := p.ArgsEntry.Values
	if len(argv) > 0 {
		argv = argv[1:]
	}
	p.Argv = argv
	p.ArgsTruncated = p.ArgsTruncated || p.ArgsEntry.Truncated
	return p.Argv, p.ArgsTruncated
}

// GetProcessArgv0 returns the first arg of the event and whether the process arguments are truncated
func (p *Process) GetProcessArgv0() (string, bool) {
	if p.ArgsEntry == nil {
		return p.Argv0, p.ArgsTruncated
	}

	argv := p.ArgsEntry.Values
	if len(argv) > 0 {
		p.Argv0 = argv[0]
	}
	p.ArgsTruncated = p.ArgsTruncated || p.ArgsEntry.Truncated
	return p.Argv0, p.ArgsTruncated
}

// Equals compares two FileFields
func (f *FileFields) Equals(o *FileFields) bool {
	return f.Inode == o.Inode && f.MountID == o.MountID && f.MTime == o.MTime && f.UID == o.UID && f.GID == o.GID && f.Mode == o.Mode
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

// IsInLowerLayer returns whether a file is in a lower layer
func (f *FileFields) IsInLowerLayer() bool {
	return f.Flags&LowerLayer != 0
}

// IsInUpperLayer returns whether a file is in the upper layer
func (f *FileFields) IsInUpperLayer() bool {
	return f.Flags&UpperLayer != 0
}

// Equals compare two FileEvent
func (e *FileEvent) Equals(o *FileEvent) bool {
	return e.FileFields.Equals(&o.FileFields)
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

// IsOverlayFS returns whether it is an overlay fs
func (e *FileEvent) IsOverlayFS() bool {
	return e.Filesystem == "overlay"
}

// MountOrigin origin of the mount
type MountOrigin = uint32

const (
	MountOriginUnknown  MountOrigin = iota // MountOriginUnknown unknown mount origin
	MountOriginProcfs                      // MountOriginProcfs mount point info from procfs
	MountOriginEvent                       // MountOriginEvent mount point info from an event
	MountOriginUnshare                     // MountOriginUnshare mount point info from an event
	MountOriginFsmount                     // MountOriginFsmount mount point info from the fsmount syscall
	MountOriginOpenTree                    // MountOriginOpenTree mount point created from the open_tree syscall
)

// MountSource source of the mount
type MountSource = uint32

const (
	MountSourceUnknown  MountSource = iota // MountSourceUnknown mount resolved from unknown source
	MountSourceMountID                     // MountSourceMountID mount resolved with the mount id
	MountSourceDevice                      // MountSourceDevice mount resolved with the device
	MountSourceSnapshot                    // MountSourceSnapshot mount resolved from the snapshot
)

// MountSources defines mount sources
var MountSources = [...]string{
	"unknown",
	"mount_id",
	"device",
	"snapshot",
}

// MountEventSource source syscall of the mount event
type MountEventSource = uint32

const (
	MountEventSourceInvalid         MountEventSource = iota // MountEventSourceInvalid the source of the mount event is invalid
	MountEventSourceMountSyscall                            // MountEventSourceMountSyscall the source of the mount event is the `mount` syscall
	MountEventSourceFsmountSyscall                          // MountEventSourceFsmountSyscall the source of the mount event is the `fsmount` syscall
	MountEventSourceOpenTreeSyscall                         // MountEventSourceOpenTreeSyscall the source of the mount event is the `open_tree` syscall
)

// MountSourceToString returns the string corresponding to a mount source
func MountSourceToString(source MountSource) string {
	return MountSources[source]
}

// MountOrigins defines mount origins
var MountOrigins = [...]string{
	"unknown",
	"procfs",
	"event",
	"unshare",
	"fsmount",
	"open_tree",
}

// MountOriginToString returns the string corresponding to a mount origin
func MountOriginToString(origin MountOrigin) string {
	return MountOrigins[origin]
}

// GetFSType returns the filesystem type of the mountpoint
func (m *Mount) GetFSType() string {
	return m.FSType
}

// IsOverlayFS returns whether it is an overlay fs
func (m *Mount) IsOverlayFS() bool {
	return m.GetFSType() == "overlay"
}

const (
	ProcessCacheEntryFromUnknown     = iota // ProcessCacheEntryFromUnknown defines a process cache entry from unknown
	ProcessCacheEntryFromPlaceholder        // ProcessCacheEntryFromPlaceholder defines the source of a placeholder process cache entry
	ProcessCacheEntryFromEvent              // ProcessCacheEntryFromEvent defines a process cache entry from event
	ProcessCacheEntryFromKernelMap          // ProcessCacheEntryFromKernelMap defines a process cache entry from kernel map
	ProcessCacheEntryFromProcFS             // ProcessCacheEntryFromProcFS defines a process cache entry from procfs. Note that some exec parent may be missing.
	ProcessCacheEntryFromSnapshot           // ProcessCacheEntryFromSnapshot defines a process cache entry from snapshot
)

// ProcessSources defines process sources
var ProcessSources = [...]string{
	"unknown",
	"placeholder",
	"event",
	"map",
	"procfs_fallback",
	"procfs_snapshot",
}

// ProcessSourceToString returns the string corresponding to a process source
func ProcessSourceToString(source uint64) string {
	return ProcessSources[source]
}

// SetTimeout updates the timeout of an activity dump
func (adlc *ActivityDumpLoadConfig) SetTimeout(duration time.Duration) {
	adlc.Timeout = duration
	adlc.EndTimestampRaw = adlc.StartTimestampRaw + uint64(duration)
}

// GetKey returns a key to uniquely identify a network device on the system
func (d NetDevice) GetKey() string {
	return fmt.Sprintf("%v_%v", d.IfIndex, d.NetNS)
}

// IsNull returns true if a key is invalid
func (p *PathKey) IsNull() bool {
	return p.Inode == 0 && p.MountID == 0
}

// PathKeySize defines the path key size
const PathKeySize = 16

// PathLeafSize defines path_leaf struct size
const PathLeafSize = PathKeySize + MaxSegmentLength + 1 + 2 + 6 // path_key + name + len + padding

// PathLeaf is the go representation of the eBPF path_leaf_t structure
type PathLeaf struct {
	Parent  PathKey
	Name    [MaxSegmentLength + 1]byte
	Len     uint16
	Padding [6]uint8
}

// GetName returns the path value as a string
func (pl *PathLeaf) GetName() string {
	return NullTerminatedString(pl.Name[:])
}

// SetName sets the path name
func (pl *PathLeaf) SetName(name string) {
	copy(pl.Name[:], []byte(name))
	pl.Len = uint16(len(name) + 1)
}

// ResolveHashes resolves the hash of the provided file
func (dfh *FakeFieldHandlers) ResolveHashes(_ EventType, _ *Process, _ *FileEvent) []string {
	return nil
}

// ResolveUserSessionContext resolves and updates the provided user session context
func (dfh *FakeFieldHandlers) ResolveUserSessionContext(_ *UserSessionContext) {}

// ResolveAWSSecurityCredentials resolves and updates the AWS security credentials of the input process entry
func (dfh *FakeFieldHandlers) ResolveAWSSecurityCredentials(_ *Event) []AWSSecurityCredentials {
	return nil
}

// ResolveSyscallCtxArgs resolves syscall context
func (dfh *FakeFieldHandlers) ResolveSyscallCtxArgs(_ *Event, _ *SyscallContext) {}

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

// ExtraFieldHandlers handlers not hold by any field
type ExtraFieldHandlers interface {
	BaseExtraFieldHandlers
	ResolveHashes(eventType EventType, process *Process, file *FileEvent) []string
	ResolveUserSessionContext(evtCtx *UserSessionContext)
	ResolveAWSSecurityCredentials(event *Event) []AWSSecurityCredentials
	ResolveSyscallCtxArgs(ev *Event, e *SyscallContext)
}
