// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors -tags unix -types-file model.go -output accessors_unix.go -field-handlers field_handlers_unix.go -doc ../../../../docs/cloud-workload-security/secl_linux.json -field-accessors-output field_accessors_unix.go

// Package model holds model related files
package model

import (
	"time"

	"modernc.org/mathutil"

	"github.com/google/gopacket"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	BaseEvent

	// globals
	Async bool `field:"event.async,handler:ResolveAsync"` // SECLDoc[event.async] Definition:`True if the syscall was asynchronous`

	// context
	SpanContext    SpanContext    `field:"-"`
	NetworkContext NetworkContext `field:"network" restricted_to:"dns,imds"` // [7.36] [Network] Network context
	CGroupContext  CGroupContext  `field:"cgroup"`

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
	Mount       MountEvent    `field:"mount" event:"mount"`             // [7.42] [File] [Experimental] A filesystem was mounted
	Chdir       ChdirEvent    `field:"chdir" event:"chdir"`             // [7.52] [File] [Experimental] A process changed the current directory

	// process events
	Exec          ExecEvent          `field:"exec" event:"exec"`     // [7.27] [Process] A process was executed or forked
	SetUID        SetuidEvent        `field:"setuid" event:"setuid"` // [7.27] [Process] A process changed its effective uid
	SetGID        SetgidEvent        `field:"setgid" event:"setgid"` // [7.27] [Process] A process changed its effective gid
	Capset        CapsetEvent        `field:"capset" event:"capset"` // [7.27] [Process] A process changed its capacity set
	Signal        SignalEvent        `field:"signal" event:"signal"` // [7.35] [Process] A signal was sent
	Exit          ExitEvent          `field:"exit" event:"exit"`     // [7.38] [Process] A process was terminated
	Syscalls      SyscallsEvent      `field:"-"`
	LoginUIDWrite LoginUIDWriteEvent `field:"-"`

	// network syscalls
	Bind    BindEvent    `field:"bind" event:"bind"`       // [7.37] [Network] A bind was executed
	Connect ConnectEvent `field:"connect" event:"connect"` // [7.60] [Network] A connect was executed

	// kernel events
	SELinux      SELinuxEvent      `field:"selinux" event:"selinux"`             // [7.30] [Kernel] An SELinux operation was run
	BPF          BPFEvent          `field:"bpf" event:"bpf"`                     // [7.33] [Kernel] A BPF command was executed
	PTrace       PTraceEvent       `field:"ptrace" event:"ptrace"`               // [7.35] [Kernel] A ptrace command was executed
	MMap         MMapEvent         `field:"mmap" event:"mmap"`                   // [7.35] [Kernel] A mmap command was executed
	MProtect     MProtectEvent     `field:"mprotect" event:"mprotect"`           // [7.35] [Kernel] A mprotect command was executed
	LoadModule   LoadModuleEvent   `field:"load_module" event:"load_module"`     // [7.35] [Kernel] A new kernel module was loaded
	UnloadModule UnloadModuleEvent `field:"unload_module" event:"unload_module"` // [7.35] [Kernel] A kernel module was deleted

	// network events
	DNS       DNSEvent       `field:"dns" event:"dns"`       // [7.36] [Network] A DNS request was sent
	IMDS      IMDSEvent      `field:"imds" event:"imds"`     // [7.55] [Network] An IMDS event was captured
	RawPacket RawPacketEvent `field:"packet" event:"packet"` // [7.60] [Network] A raw network packet captured

	// on-demand events
	OnDemand OnDemandEvent `field:"ondemand" event:"ondemand"`

	// internal usage
	Umount           UmountEvent           `field:"-"`
	InvalidateDentry InvalidateDentryEvent `field:"-"`
	ArgsEnvs         ArgsEnvsEvent         `field:"-"`
	MountReleased    MountReleasedEvent    `field:"-"`
	CgroupTracing    CgroupTracingEvent    `field:"-"`
	CgroupWrite      CgroupWriteEvent      `field:"-"`
	NetDevice        NetDeviceEvent        `field:"-"`
	VethPair         VethPairEvent         `field:"-"`
	UnshareMountNS   UnshareMountNSEvent   `field:"-"`
}

// CGroupContext holds the cgroup context of an event
type CGroupContext struct {
	CGroupID      containerutils.CGroupID    `field:"id,handler:ResolveCGroupID"` // SECLDoc[id] Definition:`ID of the cgroup`
	CGroupFlags   containerutils.CGroupFlags `field:"-"`
	CGroupManager string                     `field:"manager,handler:ResolveCGroupManager"` // SECLDoc[manager] Definition:`Lifecycle manager of the cgroup`
	CGroupFile    PathKey                    `field:"file"`
}

// SyscallEvent contains common fields for all the event
type SyscallEvent struct {
	Retval int64 `field:"retval"` // SECLDoc[retval] Definition:`Return value of the syscall` Constants:`Error constants`
}

// SyscallContext contains syscall context
type SyscallContext struct {
	ID uint32 `field:"-"`

	StrArg1 string `field:"syscall.str1,handler:ResolveSyscallCtxArgsStr1,weight:900,opts:getters_only|skip_ad"`
	StrArg2 string `field:"syscall.str2,handler:ResolveSyscallCtxArgsStr2,weight:900,opts:getters_only|skip_ad"`
	StrArg3 string `field:"syscall.str3,handler:ResolveSyscallCtxArgsStr3,weight:900,opts:getters_only|skip_ad"`

	IntArg1 int64 `field:"syscall.int1,handler:ResolveSyscallCtxArgsInt1,weight:900,opts:getters_only|skip_ad"`
	IntArg2 int64 `field:"syscall.int2,handler:ResolveSyscallCtxArgsInt2,weight:900,opts:getters_only|skip_ad"`
	IntArg3 int64 `field:"syscall.int3,handler:ResolveSyscallCtxArgsInt3,weight:900,opts:getters_only|skip_ad"`

	Resolved bool `field:"-"`
}

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	SyscallEvent
	SyscallContext
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode; file.destination.rights"` // SECLDoc[file.destination.mode] Definition:`New mode of the chmod-ed file` Constants:`File mode constants` SECLDoc[file.destination.rights] Definition:`New rights of the chmod-ed file` Constants:`File mode constants`

	// Syscall context aliases
	SyscallPath string `field:"syscall.path,ref:chmod.syscall.str1"` // SECLDoc[syscall.path] Definition:`path argument of the syscall`
	SyscallMode int64  `field:"syscall.mode,ref:chmod.syscall.int2"` // SECLDoc[syscall.mode] Definition:`mode argument of the syscall`
}

// ChownEvent represents a chown event
type ChownEvent struct {
	SyscallEvent
	SyscallContext
	File  FileEvent `field:"file"`
	UID   int64     `field:"file.destination.uid"`                           // SECLDoc[file.destination.uid] Definition:`New UID of the chown-ed file's owner`
	User  string    `field:"file.destination.user,handler:ResolveChownUID"`  // SECLDoc[file.destination.user] Definition:`New user of the chown-ed file's owner`
	GID   int64     `field:"file.destination.gid"`                           // SECLDoc[file.destination.gid] Definition:`New GID of the chown-ed file's owner`
	Group string    `field:"file.destination.group,handler:ResolveChownGID"` // SECLDoc[file.destination.group] Definition:`New group of the chown-ed file's owner`

	// Syscall context aliases
	SyscallPath string `field:"syscall.path,ref:chown.syscall.str1"` // SECLDoc[syscall.path] Definition:`Path argument of the syscall`
	SyscallUID  int64  `field:"syscall.uid,ref:chown.syscall.int2"`  // SECLDoc[syscall.uid] Definition:`UID argument of the syscall`
	SyscallGID  int64  `field:"syscall.gid,ref:chown.syscall.int3"`  // SECLDoc[syscall.gid] Definition:`GID argument of the syscall`
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

	AUID uint32 `field:"auid"` // SECLDoc[auid] Definition:`Login UID of the process`

	CapEffective uint64 `field:"cap_effective"` // SECLDoc[cap_effective] Definition:`Effective capability set of the process` Constants:`Kernel Capability constants`
	CapPermitted uint64 `field:"cap_permitted"` // SECLDoc[cap_permitted] Definition:`Permitted capability set of the process` Constants:`Kernel Capability constants`
}

// LinuxBinprm contains content from the linux_binprm struct, which holds the arguments used for loading binaries
type LinuxBinprm struct {
	FileEvent FileEvent `field:"file"`
}

// Process represents a process
type Process struct {
	PIDContext

	FileEvent FileEvent `field:"file,check:IsNotKworker"`

	CGroup      CGroupContext              `field:"cgroup"`                                         // SECLDoc[cgroup] Definition:`CGroup`
	ContainerID containerutils.ContainerID `field:"container.id,handler:ResolveProcessContainerID"` // SECLDoc[container.id] Definition:`Container ID`

	SpanID  uint64          `field:"-"`
	TraceID mathutil.Int128 `field:"-"`

	TTYName     string      `field:"tty_name"`                         // SECLDoc[tty_name] Definition:`Name of the TTY associated with the process`
	Comm        string      `field:"comm"`                             // SECLDoc[comm] Definition:`Comm attribute of the process`
	LinuxBinprm LinuxBinprm `field:"interpreter,check:HasInterpreter"` // Script interpreter as identified by the shebang

	// pid_cache_t
	ForkTime time.Time `field:"fork_time,opts:getters_only"`
	ExitTime time.Time `field:"exit_time,opts:getters_only"`
	ExecTime time.Time `field:"exec_time,opts:getters_only"`

	// TODO: merge with ExecTime
	CreatedAt uint64 `field:"created_at,handler:ResolveProcessCreatedAt"` // SECLDoc[created_at] Definition:`Timestamp of the creation of the process`

	Cookie uint64 `field:"-"`
	PPid   uint32 `field:"ppid"` // SECLDoc[ppid] Definition:`Parent process ID`

	// credentials_t section of pid_cache_t
	Credentials

	UserSession UserSessionContext `field:"user_session"` // SECLDoc[user_session] Definition:`User Session context of this process`

	AWSSecurityCredentials []AWSSecurityCredentials `field:"-"`

	ArgsID uint64 `field:"-"`
	EnvsID uint64 `field:"-"`

	ArgsEntry *ArgsEntry `field:"-"`
	EnvsEntry *EnvsEntry `field:"-"`

	// defined to generate accessors, ArgsTruncated and EnvsTruncated are used during by unmarshaller
	Argv0         string   `field:"argv0,handler:ResolveProcessArgv0,weight:100"`                                                                                                                                                                            // SECLDoc[argv0] Definition:`First argument of the process`
	Args          string   `field:"args,handler:ResolveProcessArgs,weight:500,opts:skip_ad"`                                                                                                                                                                 // SECLDoc[args] Definition:`Arguments of the process (as a string, excluding argv0)` Example:`exec.args == "-sV -p 22,53,110,143,4564 198.116.0-255.1-127"` Description:`Matches any process with these exact arguments.` Example:`exec.args =~ "* -F * http*"` Description:`Matches any process that has the "-F" argument anywhere before an argument starting with "http".`
	Argv          []string `field:"argv,handler:ResolveProcessArgv,weight:500; cmdargv,handler:ResolveProcessCmdArgv,opts:getters_only; args_flags,handler:ResolveProcessArgsFlags,opts:helper; args_options,handler:ResolveProcessArgsOptions,opts:helper"` // SECLDoc[argv] Definition:`Arguments of the process (as an array, excluding argv0)` Example:`exec.argv in ["127.0.0.1"]` Description:`Matches any process that has this IP address as one of its arguments.` SECLDoc[args_flags] Definition:`Flags in the process arguments` Example:`exec.args_flags in ["s"] && exec.args_flags in ["V"]` Description:`Matches any process with both "-s" and "-V" flags in its arguments. Also matches "-sV".` SECLDoc[args_options] Definition:`Argument of the process as options` Example:`exec.args_options in ["p=0-1024"]` Description:`Matches any process that has either "-p 0-1024" or "--p=0-1024" in its arguments.`
	ArgsTruncated bool     `field:"args_truncated,handler:ResolveProcessArgsTruncated"`                                                                                                                                                                      // SECLDoc[args_truncated] Definition:`Indicator of arguments truncation`
	Envs          []string `field:"envs,handler:ResolveProcessEnvs,weight:100"`                                                                                                                                                                              // SECLDoc[envs] Definition:`Environment variable names of the process`
	Envp          []string `field:"envp,handler:ResolveProcessEnvp,weight:100"`                                                                                                                                                                              // SECLDoc[envp] Definition:`Environment variables of the process`
	EnvsTruncated bool     `field:"envs_truncated,handler:ResolveProcessEnvsTruncated"`                                                                                                                                                                      // SECLDoc[envs_truncated] Definition:`Indicator of environment variables truncation`

	ArgsScrubbed string   `field:"args_scrubbed,handler:ResolveProcessArgsScrubbed,opts:getters_only"`
	ArgvScrubbed []string `field:"argv_scrubbed,handler:ResolveProcessArgvScrubbed,opts:getters_only"`

	// symlink to the process binary
	SymlinkPathnameStr [MaxSymlinks]string `field:"-"`
	SymlinkBasenameStr string              `field:"-"`

	// cache version
	ScrubbedArgvResolved bool           `field:"-"`
	Variables            eval.Variables `field:"-"`

	// IsThread is the negation of IsExec and should be manipulated directly
	IsThread        bool `field:"is_thread,handler:ResolveProcessIsThread"` // SECLDoc[is_thread] Definition:`Indicates whether the process is considered a thread (that is, a child process that hasn't executed another program)`
	IsExec          bool `field:"is_exec"`                                  // SECLDoc[is_exec] Definition:`Indicates whether the process entry is from a new binary execution`
	IsExecExec      bool `field:"-"`                                        // Indicates whether the process is an exec following another exec
	IsParentMissing bool `field:"-"`                                        // Indicates the direct parent is missing

	Source uint64 `field:"-"`

	// lineage
	hasValidLineage *bool `field:"-"`
	lineageError    error `field:"-"`
}

// ExecEvent represents a exec event
type ExecEvent struct {
	SyscallContext
	*Process

	// Syscall context aliases
	SyscallPath string `field:"syscall.path,ref:exec.syscall.str1"` // SECLDoc[syscall.path] Definition:`path argument of the syscall`
}

// FileFields holds the information required to identify a file
type FileFields struct {
	UID   uint32 `field:"uid"`                                           // SECLDoc[uid] Definition:`UID of the file's owner`
	User  string `field:"user,handler:ResolveFileFieldsUser"`            // SECLDoc[user] Definition:`User of the file's owner`
	GID   uint32 `field:"gid"`                                           // SECLDoc[gid] Definition:`GID of the file's owner`
	Group string `field:"group,handler:ResolveFileFieldsGroup"`          // SECLDoc[group] Definition:`Group of the file's owner`
	Mode  uint16 `field:"mode;rights,handler:ResolveRights,opts:helper"` // SECLDoc[mode] Definition:`Mode of the file` Constants:`Inode mode constants` SECLDoc[rights] Definition:`Rights of the file` Constants:`File mode constants`
	CTime uint64 `field:"change_time"`                                   // SECLDoc[change_time] Definition:`Change time (ctime) of the file`
	MTime uint64 `field:"modification_time"`                             // SECLDoc[modification_time] Definition:`Modification time (mtime) of the file`

	PathKey
	Device uint32 `field:"-"`

	InUpperLayer bool `field:"in_upper_layer,handler:ResolveFileFieldsInUpperLayer"` // SECLDoc[in_upper_layer] Definition:`Indicator of the file layer, for example, in an OverlayFS`

	NLink uint32 `field:"-"`
	Flags int32  `field:"-"`
}

// FileEvent is the common file event type
type FileEvent struct {
	FileFields

	PathnameStr string `field:"path,handler:ResolveFilePath,opts:length" op_override:"ProcessSymlinkPathname"`     // SECLDoc[path] Definition:`File's path` Example:`exec.file.path == "/usr/bin/apt"` Description:`Matches the execution of the file located at /usr/bin/apt` Example:`open.file.path == "/etc/passwd"` Description:`Matches any process opening the /etc/passwd file.`
	BasenameStr string `field:"name,handler:ResolveFileBasename,opts:length" op_override:"ProcessSymlinkBasename"` // SECLDoc[name] Definition:`File's basename` Example:`exec.file.name == "apt"` Description:`Matches the execution of any file named apt.`
	Filesystem  string `field:"filesystem,handler:ResolveFileFilesystem"`                                          // SECLDoc[filesystem] Definition:`File's filesystem`

	MountPath   string `field:"-"`
	MountSource uint32 `field:"-"`
	MountOrigin uint32 `field:"-"`

	PathResolutionError error `field:"-"`

	PkgName       string `field:"package.name,handler:ResolvePackageName"`                    // SECLDoc[package.name] Definition:`[Experimental] Name of the package that provided this file`
	PkgVersion    string `field:"package.version,handler:ResolvePackageVersion"`              // SECLDoc[package.version] Definition:`[Experimental] Full version of the package that provided this file`
	PkgSrcVersion string `field:"package.source_version,handler:ResolvePackageSourceVersion"` // SECLDoc[package.source_version] Definition:`[Experimental] Full version of the source package of the package that provided this file`

	HashState HashState `field:"-"`
	Hashes    []string  `field:"hashes,handler:ResolveHashesFromEvent,opts:skip_ad,weight:999"` // SECLDoc[hashes] Definition:`[Experimental] List of cryptographic hashes computed for this file`

	// used to mark as already resolved, can be used in case of empty path
	IsPathnameStrResolved bool `field:"-"`
	IsBasenameStrResolved bool `field:"-"`
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
	SyscallContext
	Source FileEvent `field:"file"`
	Target FileEvent `field:"file.destination"`

	// Syscall context aliases
	SyscallPath            string `field:"syscall.path,ref:link.syscall.str1"`             // SECLDoc[syscall.path] Definition:`Path argument of the syscall`
	SyscallDestinationPath string `field:"syscall.destination.path,ref:link.syscall.str2"` // SECLDoc[syscall.destination.path] Definition:`Destination path argument of the syscall`
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode; file.destination.rights"` // SECLDoc[file.destination.mode] Definition:`Mode of the new directory` Constants:`File mode constants` SECLDoc[file.destination.rights] Definition:`Rights of the new directory` Constants:`File mode constants`
}

// ArgsEnvsEvent defines a args/envs event
type ArgsEnvsEvent struct {
	ArgsEnvs
}

// Mount represents a mountpoint (used by MountEvent and UnshareMountNSEvent)
type Mount struct {
	MountID        uint32  `field:"-"`
	Device         uint32  `field:"-"`
	ParentPathKey  PathKey `field:"-"`
	RootPathKey    PathKey `field:"-"`
	BindSrcMountID uint32  `field:"-"`
	FSType         string  `field:"fs_type"` // SECLDoc[fs_type] Definition:`Type of the mounted file system`
	MountPointStr  string  `field:"-"`
	RootStr        string  `field:"-"`
	Path           string  `field:"-"`
	Origin         uint32  `field:"-"`
}

// MountEvent represents a mount event
type MountEvent struct {
	SyscallEvent
	SyscallContext
	Mount
	MountPointPath                 string `field:"mountpoint.path,handler:ResolveMountPointPath"` // SECLDoc[mountpoint.path] Definition:`Path of the mount point`
	MountSourcePath                string `field:"source.path,handler:ResolveMountSourcePath"`    // SECLDoc[source.path] Definition:`Source path of a bind mount`
	MountRootPath                  string `field:"root.path,handler:ResolveMountRootPath"`        // SECLDoc[root.path] Definition:`Root path of the mount`
	MountPointPathResolutionError  error  `field:"-"`
	MountSourcePathResolutionError error  `field:"-"`
	MountRootPathResolutionError   error  `field:"-"`

	// Syscall context aliases
	SyscallSourcePath     string `field:"syscall.source.path,ref:mount.syscall.str1"`     // SECLDoc[syscall.source.path] Definition:`Source path argument of the syscall`
	SyscallMountpointPath string `field:"syscall.mountpoint.path,ref:mount.syscall.str2"` // SECLDoc[syscall.mountpoint.path] Definition:`Mount point path argument of the syscall`
	SyscallFSType         string `field:"syscall.fs_type,ref:mount.syscall.str3"`         // SECLDoc[syscall.fs_type] Definition:`File system type argument of the syscall`
}

// UnshareMountNSEvent represents a mount cloned from a newly created mount namespace
type UnshareMountNSEvent struct {
	Mount
}

// ChdirEvent represents a chdir event
type ChdirEvent struct {
	SyscallEvent
	SyscallContext
	File FileEvent `field:"file"`

	// Syscall context aliases
	SyscallPath string `field:"syscall.path,ref:chdir.syscall.str1"` // SECLDoc[syscall.path] Definition:`path argument of the syscall`
}

// OpenEvent represents an open event
type OpenEvent struct {
	SyscallEvent
	SyscallContext
	File  FileEvent `field:"file"`
	Flags uint32    `field:"flags"`                 // SECLDoc[flags] Definition:`Flags used when opening the file` Constants:`Open flags`
	Mode  uint32    `field:"file.destination.mode"` // SECLDoc[file.destination.mode] Definition:`Mode of the created file` Constants:`File mode constants`

	// Syscall context aliases
	SyscallPath  string `field:"syscall.path,ref:open.syscall.str1"`  // SECLDoc[syscall.path] Definition:`Path argument of the syscall`
	SyscallFlags uint32 `field:"syscall.flags,ref:open.syscall.int2"` // SECLDoc[syscall.flags] Definition:`Flags argument of the syscall`
	SyscallMode  uint32 `field:"syscall.mode,ref:open.syscall.int3"`  // SECLDoc[syscall.mode] Definition:`Mode argument of the syscall`
}

// SELinuxEvent represents a selinux event
type SELinuxEvent struct {
	File            FileEvent        `field:"-"`
	EventKind       SELinuxEventKind `field:"-"`
	BoolName        string           `field:"bool.name,handler:ResolveSELinuxBoolName"` // SECLDoc[bool.name] Definition:`SELinux boolean name`
	BoolChangeValue string           `field:"bool.state"`                               // SECLDoc[bool.state] Definition:`SELinux boolean new value`
	BoolCommitValue bool             `field:"bool_commit.state"`                        // SECLDoc[bool_commit.state] Definition:`Indicator of a SELinux boolean commit operation`
	EnforceStatus   string           `field:"enforce.status"`                           // SECLDoc[enforce.status] Definition:`SELinux enforcement status (one of "enforcing", "permissive", "disabled")`
}

// PIDContext holds the process context of a kernel event
type PIDContext struct {
	Pid       uint32 `field:"pid"` // SECLDoc[pid] Definition:`Process ID of the process (also called thread group ID)`
	Tid       uint32 `field:"tid"` // SECLDoc[tid] Definition:`Thread ID of the thread`
	NetNS     uint32 `field:"-"`
	IsKworker bool   `field:"is_kworker"` // SECLDoc[is_kworker] Definition:`Indicates whether the process is a kworker`
	ExecInode uint64 `field:"-"`          // used to track exec and event loss
	// used for ebpfless
	NSID uint64 `field:"-"`
}

// RenameEvent represents a rename event
type RenameEvent struct {
	SyscallEvent
	SyscallContext
	Old FileEvent `field:"file"`
	New FileEvent `field:"file.destination"`

	// Syscall context aliases
	SyscallPath            string `field:"syscall.path,ref:rename.syscall.str1"`             // SECLDoc[syscall.path] Definition:`Path argument of the syscall`
	SyscallDestinationPath string `field:"syscall.destination.path,ref:rename.syscall.str2"` // SECLDoc[syscall.destination.path] Definition:`Destination path argument of the syscall`
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

	NameRaw [200]byte `field:"-"`
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	SyscallEvent
	SyscallContext
	File  FileEvent `field:"file"`
	Flags uint32    `field:"flags"` // SECLDoc[flags] Definition:`Flags of the unlink syscall` Constants:`Unlink flags`

	// Syscall context aliases
	SyscallDirFd uint64 `field:"syscall.dirfd,ref:unlink.syscall.int1"` // SECLDoc[syscall.dirfd] Definition:`Directory file descriptor argument of the syscall`
	SyscallPath  string `field:"syscall.path,ref:unlink.syscall.str2"`  // SECLDoc[syscall.path] Definition:`Path argument of the syscall`
	SyscallFlags uint64 `field:"syscall.flags,ref:unlink.syscall.int3"` // SECLDoc[syscall.flags] Definition:`Flags argument of the syscall`
}

// UmountEvent represents an umount event
type UmountEvent struct {
	SyscallEvent
	MountID uint32
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	SyscallEvent
	SyscallContext
	File  FileEvent `field:"file"`
	Atime time.Time `field:"-"`
	Mtime time.Time `field:"-"`

	// Syscall context aliases
	SyscallPath string `field:"syscall.path,ref:utimes.syscall.str1"` // SECLDoc[syscall.path] Definition:`Path argument of the syscall`
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
	ID   uint32 `field:"-"`    // ID of the eBPF map
	Type uint32 `field:"type"` // SECLDoc[type] Definition:`Type of the eBPF map` Constants:`BPF map types`
	Name string `field:"name"` // SECLDoc[name] Definition:`Name of the eBPF map (added in 7.35)`
}

// BPFProgram represents a BPF program
type BPFProgram struct {
	ID         uint32   `field:"-"`           // ID of the eBPF program
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
	PID     uint32          `field:"-"`
	Address uint64          `field:"-"`
	Tracee  *ProcessContext `field:"tracee"` // process context of the tracee
}

// MMapEvent represents a mmap event
type MMapEvent struct {
	SyscallEvent

	File       FileEvent `field:"file"`
	Addr       uint64    `field:"-"`
	Offset     uint64    `field:"-"`
	Len        uint64    `field:"-"`
	Protection uint64    `field:"protection"` // SECLDoc[protection] Definition:`memory segment protection` Constants:`Protection constants`
	Flags      uint64    `field:"flags"`      // SECLDoc[flags] Definition:`memory segment flags` Constants:`MMap flags`
}

// MProtectEvent represents a mprotect event
type MProtectEvent struct {
	SyscallEvent

	VMStart       uint64 `field:"-"`
	VMEnd         uint64 `field:"-"`
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
	CGroupContext    CGroupContext
	Config           ActivityDumpLoadConfig
	ConfigCookie     uint64
}

// CgroupWriteEvent is used to signal that a new cgroup was created
type CgroupWriteEvent struct {
	File        FileEvent `field:"file"` // Path to the cgroup
	Pid         uint32    `field:"-"`    // PID of the process added to the cgroup
	CGroupFlags uint32    `field:"-"`    // CGroup flags
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

// NetworkDeviceContext represents the network device context of a network event
type NetworkDeviceContext struct {
	NetNS   uint32 `field:"-"`
	IfIndex uint32 `field:"-"`
	IfName  string `field:"ifname,handler:ResolveNetworkDeviceIfName"` // SECLDoc[ifname] Definition:`Interface ifname`
}

// BindEvent represents a bind event
type BindEvent struct {
	SyscallEvent

	Addr       IPPortContext `field:"addr"`        // Bound address
	AddrFamily uint16        `field:"addr.family"` // SECLDoc[addr.family] Definition:`Address family`
}

// ConnectEvent represents a connect event
type ConnectEvent struct {
	SyscallEvent
	Addr       IPPortContext `field:"addr;server.addr"`               // Connection address
	AddrFamily uint16        `field:"addr.family;server.addr.family"` // SECLDoc[addr.family] Definition:`Address family` SECLDoc[server.addr.family] Definition:`Server address family`
}

// NetDevice represents a network device
type NetDevice struct {
	Name        string
	NetNS       uint32
	IfIndex     uint32
	PeerNetNS   uint32
	PeerIfIndex uint32
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
	EventReason SyscallDriftEventReason
	Syscalls    []Syscall // 64 * 8 = 512 > 450, bytes should be enough to hold all 450 syscalls
}

// PathKey identifies an entry in the dentry cache
type PathKey struct {
	Inode   uint64 `field:"inode"`    // SECLDoc[inode] Definition:`Inode of the file`
	MountID uint32 `field:"mount_id"` // SECLDoc[mount_id] Definition:`Mount ID of the file`
	PathID  uint32 `field:"-"`
}

// OnDemandEvent identifies an on-demand event generated from on-demand probes
type OnDemandEvent struct {
	ID       uint32    `field:"-"`
	Name     string    `field:"name,handler:ResolveOnDemandName"`
	Data     [256]byte `field:"-"`
	Arg1Str  string    `field:"arg1.str,handler:ResolveOnDemandArg1Str"`
	Arg1Uint uint64    `field:"arg1.uint,handler:ResolveOnDemandArg1Uint"`
	Arg2Str  string    `field:"arg2.str,handler:ResolveOnDemandArg2Str"`
	Arg2Uint uint64    `field:"arg2.uint,handler:ResolveOnDemandArg2Uint"`
	Arg3Str  string    `field:"arg3.str,handler:ResolveOnDemandArg3Str"`
	Arg3Uint uint64    `field:"arg3.uint,handler:ResolveOnDemandArg3Uint"`
	Arg4Str  string    `field:"arg4.str,handler:ResolveOnDemandArg4Str"`
	Arg4Uint uint64    `field:"arg4.uint,handler:ResolveOnDemandArg4Uint"`
}

// LoginUIDWriteEvent is used to propagate login UID updates to user space
type LoginUIDWriteEvent struct {
	AUID uint32 `field:"-"`
}

// RawPacketEvent represents a packet event
type RawPacketEvent struct {
	NetworkContext
	TLSContext  TLSContext           `field:"tls"`                                       // SECLDoc[tls] Definition:`TLS context`
	Filter      string               `field:"filter" op_override:"PacketFilterMatching"` // SECLDoc[filter] Definition:`pcap filter expression`
	CaptureInfo gopacket.CaptureInfo `field:"-"`
	Data        []byte               `field:"-"`
}
