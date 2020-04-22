package model

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"docker.io/go-docker/api/types"
	"docker.io/go-docker/api/types/container"
	"docker.io/go-docker/api/types/strslice"
	"github.com/docker/go-connections/nat"
)

// EventMonitorName - Event Monitor names
type EventMonitorName string

const (
	// FimMonitor - eBPF FIM probe
	FimMonitor EventMonitorName = "fim"
	// ProcessMonitor - eBPF Process probe
	ProcessMonitor EventMonitorName = "process"
)

// EventMonitorType - Probe type
type EventMonitorType string

const (
	// EBPF - eBPF probe
	EBPF EventMonitorType = "ebpf"
	// Perf - Perf probe
	Perf EventMonitorType = "perf"
)

// ProbeEventType - ProbeEventType enum
type ProbeEventType string

const (
	// Internal events

	// UnknownEventType - Dummy event to handle errors
	UnknownEventType ProbeEventType = "Unknown"
	// DelayedEventType - This event is used to register the Delayer with the probe manager. There
	// 2 kinds of delayed events. The first one happens when a context aware filter is set and hasn't
	// been resolved into a kernel filter yet. The other one happens when the security module decides
	// to requeue an events because it couldn't be assessed at this time.
	DelayedEventType ProbeEventType = "Unfiltered"
	// FilterResolutionEventType - This event is used by the probe manager to notify the delayer
	// that a new filter has been resolved. Thus asking it to either destroy or requeue the delayed
	// events
	FilterResolutionEventType ProbeEventType = "FilterResolution"
	// SecurityResolutionEventType - This event is used by the security module to signal that a
	// new process / container has been resolved, and that queued events for this process /
	// container can now be handled (thus asking the delayer to dispatch them again).
	SecurityResolutionEventType ProbeEventType = "SecurityResolution"

	// BASHSNOOP events

	// BashReadlineEventType - Event type for a bash readline event
	BashReadlineEventType ProbeEventType = "BashReadline"
	// BashTTYWriteEventType - Event type for a bash tty write event
	BashTTYWriteEventType ProbeEventType = "BashTTYWrite"

	// PROCESS events

	// ProcessExecEventType - Event type for process creation
	ProcessExecEventType ProbeEventType = "ProcessExec"
	// ProcessExitEventType - Event type for process exit
	ProcessExitEventType ProbeEventType = "ProcessExit"
	// ProcessCredsCommitEventType - Event type for a credentials commit event
	ProcessCredsCommitEventType ProbeEventType = "ProcessCredCommit"
	// ProcessForkEventType - Event type for a fork event
	ProcessForkEventType ProbeEventType = "ProcessFork"
	// ProcessCwdEventType - Event type for a cwd event
	ProcessCwdEventType ProbeEventType = "ProcessCwd"
	// ProcessNamespaceEnterEventType - Event type for a setns enter event
	ProcessNamespaceEnterEventType ProbeEventType = "ProcessNamespaceEnter"
	// ProcessNamespaceExitEventType - Event type for a setns exit event
	ProcessNamespaceExitEventType ProbeEventType = "ProcessNamespaceExit"

	// FIM events

	// FileOpenEventType - File open event
	FileOpenEventType ProbeEventType = "FileOpen"
	// FileMkdirEventType - Folder creation event
	FileMkdirEventType ProbeEventType = "FileMkdir"
	// FileHardLinkEventType - Hard link creation event
	FileHardLinkEventType ProbeEventType = "FileHardLink"
	// FileRenameEventType - File or folder rename event
	FileRenameEventType ProbeEventType = "FileRename"
	// FileSetAttrEventType - Set Attr event
	FileSetAttrEventType ProbeEventType = "FileSetAttr"
	// FileUnlinkEventType - Unlink event
	FileUnlinkEventType ProbeEventType = "FileUnlink"
	// FileRmdirEventType - Rmdir event
	FileRmdirEventType ProbeEventType = "FileRmdir"
	// FileModifyEventType - File modify event
	FileModifyEventType ProbeEventType = "FileModify"

	// SYSCALL events

	// SyscallEventType - Event type for a syscall event
	SyscallEventType ProbeEventType = "Syscall"

	// NETWORK events

	// NetworkNetDevXmitEventType - Event type for a network net_dev_xmit event
	NetworkNetDevXmitEventType ProbeEventType = "NetworkNetDevXmit"
	// NetworkConsumeSKBEventType - Event type for a network consume_skb event
	NetworkConsumeSKBEventType ProbeEventType = "NetworkConsumeSKB"
	// NetworkCopyDatagramIovecEventType - Event type for a network copy_datagram_iovec event
	NetworkCopyDatagramIovecEventType ProbeEventType = "NetworkCopyDatagramIovec"

	// SOCKET events

	// SocketCreateEventType - Event type for a socket creation event
	SocketCreateEventType ProbeEventType = "SocketCreate"
	// SocketBindEventType - Event type for a bind event
	SocketBindEventType ProbeEventType = "SocketBind"
	// SocketAcceptEventType - Event type for a accept event
	SocketAcceptEventType ProbeEventType = "SocketAccept"
	// SocketConnectEventType - Event type for a connect event
	SocketConnectEventType ProbeEventType = "SocketConnect"
	// SocketListenEventType - Event type for a listen event
	SocketListenEventType ProbeEventType = "SocketListen"
	// SocketCloseEventType - Event type for a close event
	SocketCloseEventType ProbeEventType = "SocketClose"

	// CONTAINER event

	// ContainerCreatedEventType - Event type for a container creation event
	ContainerCreatedEventType ProbeEventType = "ContainerCreated"
	// ContainerRunningEventType - Event type for a running container event
	ContainerRunningEventType ProbeEventType = "ContainerRunning"
	// ContainerExitedEventType - Event type for a container exit event
	ContainerExitedEventType ProbeEventType = "ContainerExit"
	// ContainerDestroyedEventType - Event type for a container destroy event
	ContainerDestroyedEventType ProbeEventType = "ContainerDestroyed"
	// ContainerExecEventType - Event type for a container exec event
	ContainerExecEventType ProbeEventType = "ContainerExec"
	// ContainerAttachEventType - Event type for a container attach event
	ContainerAttachEventType ProbeEventType = "ContainerAttach"
	// ContainerConnectEventType - Event type for a container connect event
	ContainerConnectEventType ProbeEventType = "ContainerConnect"
	// ContainerDisconnectEventType - Event type for a container disconnect event
	ContainerDisconnectEventType ProbeEventType = "ContainerDisconnect"
)

// ProbeEvent - Probe event interface
type ProbeEvent interface {
	GetPid() uint32
	GetPidns() uint64
	GetUID() int
	GetGID() int
	GetMountIDs() []int
	GetTimestamp() time.Time
	GetEventType() ProbeEventType
	GetEventMonitorName() EventMonitorName
	GetEventMonitorType() EventMonitorType
	GetRoutingFlag() EventRoutingFlag
	SetRoutingFlag(flag EventRoutingFlag)
	AddRoutingFlag(flag EventRoutingFlag)
	RemoveRoutingFlag(flag EventRoutingFlag)
	HasRoutingFlag(flag EventRoutingFlag) bool
	SetProcessCacheData(entry *ProcessCacheEntry)
	GetProcessCacheData() *ProcessCacheEntry
	SetNamespaceCacheData(entry *NamespaceCacheEntry)
	GetNamespaceCacheData() *NamespaceCacheEntry
	AddMountCacheData(entry *MountCacheEntry)
	GetMountCacheData() []*MountCacheEntry
	SetUserCacheData(entry *UserCacheEntry)
	GetUserCacheData() *UserCacheEntry
	GetMountPointPath(mountID int) string
}

// ProcessCacheEntry - Process cache entry
type ProcessCacheEntry struct {
	sync.RWMutex
	IsUnexpectedProcess  bool       `json:"is_unexpected_process" field:"is_unexpected_process"`
	ForkThresholdReached bool       `json:"fork_threshold_reached" field:"fork_threshold_reached"`
	BinaryPath           string     `json:"binary_path" field:"binary"`
	Comm                 string     `json:"comm,omitempty" field:"comm"`
	Ppid                 uint32     `json:"ppid" field:"ppid"`
	Pid                  uint32     `json:"pid" field:"pid"`
	TTYName              string     `json:"tty_name,omitempty" field:"tty"`
	ExecveTime           *time.Time `json:"execve_time"`
	ForkTime             *time.Time `json:"fork_time"`
	ExitTime             *time.Time `json:"-"`
}

func (pce *ProcessCacheEntry) String() string {
	printStr := ""
	if pce.BinaryPath != "" {
		printStr = fmt.Sprintf("binary_path:%v", pce.BinaryPath)
	} else {
		printStr = fmt.Sprintf("command:%v", pce.Comm)
	}
	if len(pce.TTYName) > 0 {
		printStr = fmt.Sprintf("%v tty:%v", printStr, pce.TTYName)
	}
	return printStr
}

// MultiprocessingThreshold - Multiprocessing threshold
var MultiprocessingThreshold = 100 * time.Millisecond

// IsExecveResolved - Checks if the execve & fork times are consistent to declare that
// the process and profile that are set in the current cacheEntry are the real process
// data. In other words this functions guesses if the process crossed the threshold to
// be considered as a multiprocessed or if we should wait to make sure that no another
// is on its way.
func (pce *ProcessCacheEntry) IsExecveResolved(timestamp time.Time, updateState bool) bool {
	if updateState {
		pce.Lock()
		defer pce.Unlock()
	} else {
		pce.RLock()
		defer pce.RUnlock()
	}
	if pce.ForkTime == nil || pce.ForkThresholdReached {
		return true
	}
	if pce.ExecveTime != nil && pce.ForkTime.Before(*pce.ExecveTime) {
		if updateState {
			pce.ForkThresholdReached = true
		}
		return true
	}
	if pce.ForkTime.Add(MultiprocessingThreshold).Before(timestamp) {
		if updateState {
			pce.ForkThresholdReached = true
		}
		return true
	}
	return false
}

// HasQuickExitTime - Checks if the exit time is below the fork threshold
func (pce *ProcessCacheEntry) HasQuickExitTime() bool {
	if pce.ExitTime == nil || pce.ForkTime == nil {
		return false
	}
	return pce.ForkTime.Add(MultiprocessingThreshold).After(*pce.ExitTime)
}

// IsInCache - Checks if the process is in cache
func (pce *ProcessCacheEntry) IsInCache() bool {
	pce.RLock()
	defer pce.RUnlock()
	inCache := pce.ExecveTime != nil || pce.ForkTime != nil
	return inCache
}

// NamespaceCacheEntry - Namespace cache entry
type NamespaceCacheEntry struct {
	sync.RWMutex
	IsUnexpectedNamespace bool       `json:"is_unexpected_namespace" field:"is_unexpected_namespace"`
	Name                  string     `json:"name" field:"name"`
	ID                    string     `json:"id" field:"id"`
	Base                  string     `json:"base" field:"base"`
	Digest                string     `json:"digest" field:"digest"`
	ExitTime              *time.Time `json:"-"`
}

func (nce *NamespaceCacheEntry) String() string {
	return fmt.Sprintf(
		"namespace:%v",
		nce.Name,
	)
}

// IsInCache - Checks if a namespace entry is in cache
func (nce *NamespaceCacheEntry) IsInCache() bool {
	nce.RLock()
	inCache := len(nce.Name) > 0
	nce.RUnlock()
	return inCache
}

// InodeCacheEntry - Inode cache entry
type InodeCacheEntry struct {
	Path string `json:"path"`
}

// MountCacheEntry - Mount cache entry
type MountCacheEntry struct {
	//*utils.MountInfo
}

func (mce *MountCacheEntry) String() string {
	/*return fmt.Sprintf(
		"mount_id:%v fstype:%v source:%v mount_point:%v",
		mce.MountID,
		mce.FSType,
		mce.Source,
		mce.MountPoint,
	)*/
	return ""
}

// UserCacheEntry - User cache entry
type UserCacheEntry struct {
	//UserData  *utils.EtcPasswdEntry
	//GroupData *utils.GroupEntry
	// TODO: add last login
}

func (uce *UserCacheEntry) String() string {
	printStr := ""
	/*if uce.UserData != nil {
		printStr = fmt.Sprintf("user:%v", uce.UserData.Username)
	}
	if uce.GroupData != nil {
		printStr = fmt.Sprintf("%v group:%v", printStr, uce.GroupData.GroupName)
	}*/
	return printStr
}

// EventRoutingFlag - Internal routing flag
type EventRoutingFlag uint8

const (
	// EmptyFlag - Empty flag used to reset the routing flag
	EmptyFlag EventRoutingFlag = 0
	// UnfilteredEventFlag - This event wasn't filtered and should be delayed until the filter can be applied
	// This usually happens when a `binary` or `container` or `image` filter was provided. Until those filters
	// are resolved to a `pid` or a `pidns`, any captured event is considered as unfiltered.
	UnfilteredEventFlag EventRoutingFlag = 1 << 0
	// SecurityDelayedEventFlag - This event was delayed by the security module because it wasn't able to make a
	// decision based on it's cache.
	SecurityDelayedEventFlag EventRoutingFlag = 1 << 1
	// CacheDataFlag - This flag means that this event shouldn't be routed to the subscribers, its only purpose
	// is to improve the data cache and filtering and / or to clear unfiltered events in the delayer.
	CacheDataFlag EventRoutingFlag = 1 << 2
)

// HasRoutingFlag - Checks if a routing flag is present
func (erf EventRoutingFlag) HasRoutingFlag(flag EventRoutingFlag) bool {
	return erf&flag == flag
}

// EventBase - Base struct for a probe event
type EventBase struct {
	ProcessData       *ProcessCacheEntry   `json:"process_data,omitempty" field:"process"`
	NamespaceData     *NamespaceCacheEntry `json:"namespace_data,omitempty" field:"namespace"`
	MountData         []*MountCacheEntry   `json:"mount_data,omitempty" field:"-"`
	UserData          *UserCacheEntry      `json:"user_data,omitempty" field:"-"`
	HasSecurityAlerts bool                 `json:"has_security_alerts" field:"has_security_alerts"`
	RoutingFlag       EventRoutingFlag     `json:"-"`
	EventType         ProbeEventType       `json:"event_type" field:"type"`
	// EventMonitorName  EventMonitorName     `json:"event_monitor_name" field:"monitor"`
	// EventMonitorType  EventMonitorType     `json:"event_monitor_type" field:"monitor_type"`
	Timestamp time.Time `json:"timestamp"`
}

// GetTimestamp - Returns the event timestamp
func (eb *EventBase) GetTimestamp() time.Time {
	return eb.Timestamp
}

// GetEventType - Returns the event type
func (eb *EventBase) GetEventType() ProbeEventType {
	return eb.EventType
}

// GetEventMonitorName - Returns the event monitor name
/*
func (eb *EventBase) GetEventMonitorName() EventMonitorName {
	return eb.EventMonitorName
}

// GetEventMonitorType - Returns the event monitor Type
func (eb *EventBase) GetEventMonitorType() EventMonitorType {
	return eb.EventMonitorType
}
*/

// SetProcessCacheData - Sets the process cache data
func (eb *EventBase) SetProcessCacheData(pce *ProcessCacheEntry) {
	eb.ProcessData = pce
}

// GetProcessCacheData - Returns the process cache data
func (eb *EventBase) GetProcessCacheData() *ProcessCacheEntry {
	return eb.ProcessData
}

// SetNamespaceCacheData - Sets the namespace cache data
func (eb *EventBase) SetNamespaceCacheData(nce *NamespaceCacheEntry) {
	eb.NamespaceData = nce
}

// GetNamespaceCacheData - Returns the namespace cache data
func (eb *EventBase) GetNamespaceCacheData() *NamespaceCacheEntry {
	return eb.NamespaceData
}

// GetRoutingFlag - Returns the event routing flag
func (eb *EventBase) GetRoutingFlag() EventRoutingFlag {
	return eb.RoutingFlag
}

// SetRoutingFlag - Sets the event routing flag
func (eb *EventBase) SetRoutingFlag(flag EventRoutingFlag) {
	eb.RoutingFlag = flag
}

// AddRoutingFlag - Add routing flag to the existing one
func (eb *EventBase) AddRoutingFlag(flag EventRoutingFlag) {
	eb.RoutingFlag = eb.RoutingFlag | flag
}

// RemoveRoutingFlag - Removes a routing flag
func (eb *EventBase) RemoveRoutingFlag(flag EventRoutingFlag) {
	eb.RoutingFlag = eb.RoutingFlag &^ flag
}

// HasRoutingFlag - Checks if a routing flag is set
func (eb *EventBase) HasRoutingFlag(flag EventRoutingFlag) bool {
	return eb.RoutingFlag&flag == flag
}

// AddMountCacheData - Sets the mount cache data
func (eb *EventBase) AddMountCacheData(entry *MountCacheEntry) {
	eb.MountData = append(eb.MountData, entry)
}

// GetMountCacheData - Returns the mount cache data
func (eb *EventBase) GetMountCacheData() []*MountCacheEntry {
	return eb.MountData
}

// SetUserCacheData - Sets the user cache data
func (eb *EventBase) SetUserCacheData(entry *UserCacheEntry) {
	eb.UserData = entry
}

// GetUserCacheData - Returns the user cache data
func (eb *EventBase) GetUserCacheData() *UserCacheEntry {
	return eb.UserData
}

// GetMountPointPath - Returns the full resolved path of the mount point ID (resolved to the host fs)
func (eb *EventBase) GetMountPointPath(mountID int) string {
	var mountPath string
	/*var mount *MountCacheEntry
	for _, mnt := range eb.GetMountCacheData() {
		if mnt.MountID == mountID {
			mount = mnt
		}
	}
	if mount == nil {
		return ""
	}
	mountPath = mount.MountPoint
	if mount.FSType == "overlay" {
		for _, mnt := range eb.GetMountCacheData() {
			if mnt.MajorMinorVer == mount.MajorMinorVer {
				mountPath = path.Join(mnt.MountPoint, mountPath)
			}
		}
	}*/
	return mountPath
}

// SecurityResolutionEvent - Security resolution event
type SecurityResolutionEvent struct {
	EventBase
	Pidns uint64 `json:"pidns"`
	Pid   uint32 `json:"pid"`
}

// NewSecurityResolutionEvent - Creates a new security resolution event
func NewSecurityResolutionEvent(pidns uint64, pid uint32) SecurityResolutionEvent {
	return SecurityResolutionEvent{
		EventBase: EventBase{
			EventType: SecurityResolutionEventType,
		},
		Pid:   pid,
		Pidns: pidns,
	}
}

// GetUID - Returns the event UID
func (sre *SecurityResolutionEvent) GetUID() int {
	return -1
}

// GetGID - Returns the event GID
func (sre *SecurityResolutionEvent) GetGID() int {
	return -1
}

// GetMountIDs - Returns the event Mount ID
func (sre *SecurityResolutionEvent) GetMountIDs() []int {
	return []int{}
}

// GetPid - Returns the event pid
func (sre *SecurityResolutionEvent) GetPid() uint32 {
	return sre.Pid
}

// GetPidns - Returns the event pidns
func (sre *SecurityResolutionEvent) GetPidns() uint64 {
	return sre.Pidns
}

func (sre *SecurityResolutionEvent) String() string {
	return fmt.Sprintf(
		"pid:%v pidns:%v",
		sre.Pid,
		sre.Pidns,
	)
}

// BashReadlineEventRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type BashReadlineEventRaw struct {
	Pidns        uint64     `json:"pidns"`
	Pid          uint32     `json:"pid"`
	Tid          uint32     `json:"tid"`
	UID          uint32     `json:"-"`
	GID          uint32     `json:"-"`
	TimestampRaw uint64     `json:"-"`
	TTYNameRaw   [64]byte   `json:"-"`
	CmdRaw       [4096]byte `json:"-"`
}

// BashReadlineEvent - Bashsnoop event definition
type BashReadlineEvent struct {
	EventBase
	*BashReadlineEventRaw
	TTYName string `json:"tty_name"`
	Cmd     string `json:"cmd"`
}

// GetUID - Returns the event UID
func (bs *BashReadlineEvent) GetUID() int {
	return int(bs.UID)
}

// GetGID - Returns the event GID
func (bs *BashReadlineEvent) GetGID() int {
	return int(bs.GID)
}

// GetMountIDs - Returns the event Mount ID
func (bs *BashReadlineEvent) GetMountIDs() []int {
	return []int{}
}

// GetPid - Returns the event pid
func (bs *BashReadlineEvent) GetPid() uint32 {
	return bs.Pid
}

// GetPidns - Returns the event pidns
func (bs *BashReadlineEvent) GetPidns() uint64 {
	return bs.Pidns
}

func (bs *BashReadlineEvent) String() string {
	return fmt.Sprintf(
		"pid:%v tid:%v pidns:%v cmd:\"%v\"",
		bs.Pid,
		bs.Tid,
		bs.Pidns,
		bs.Cmd,
	)
}

// TTYWriteEventRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type TTYWriteEventRaw struct {
	Pidns        uint64     `json:"pidns"`
	Pid          uint32     `json:"pid"`
	Tid          uint32     `json:"tid"`
	UID          uint32     `json:"-"`
	GID          uint32     `json:"-"`
	TimestampRaw uint64     `json:"-"`
	TTYNameRaw   [64]byte   `json:"-"`
	ByteCount    uint64     `json:"byte_count"`
	OutputRaw    [4096]byte `json:"-"`
}

// TTYWriteEvent - TTYWrite event definition
type TTYWriteEvent struct {
	EventBase
	*TTYWriteEventRaw
	TTYName string `json:"tty_name"`
	Output  string `json:"output"`
}

// GetUID - Returns the event UID
func (ttyw *TTYWriteEvent) GetUID() int {
	return int(ttyw.UID)
}

// GetGID - Returns the event GID
func (ttyw *TTYWriteEvent) GetGID() int {
	return int(ttyw.GID)
}

// GetMountIDs - Returns the event Mount ID
func (ttyw *TTYWriteEvent) GetMountIDs() []int {
	return []int{}
}

// GetPid - Returns the event pid
func (ttyw *TTYWriteEvent) GetPid() uint32 {
	return ttyw.Pid
}

// GetPidns - Returns the event pidns
func (ttyw *TTYWriteEvent) GetPidns() uint64 {
	return ttyw.Pidns
}

func (ttyw *TTYWriteEvent) String() string {
	return fmt.Sprintf(
		"pid:%v tid:%v pidns:%v tty:%v byte_count:%v output:\"\n====================\n%v====================\n\"",
		ttyw.Pid,
		ttyw.Tid,
		ttyw.Pidns,
		ttyw.TTYName,
		ttyw.ByteCount,
		ttyw.Output,
	)
}

// DentryEventRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type DentryEventRaw struct {
	Pidns             uint64   `json:"pidns" field:"pidns"`
	TimestampRaw      uint64   `json:"-" field:"-"`
	TTYNameRaw        [64]byte `json:"-" field:"-"`
	Pid               uint32   `json:"pid" field:"pid"`
	Tid               uint32   `json:"tid" field:"tid"`
	UID               uint32   `json:"uid" field:"uid"`
	GID               uint32   `json:"gid" field:"gid"`
	Flags             int32    `json:"flags,omitempty" field:"flags"`
	Mode              int32    `json:"mode,omitempty" field:"mode"`
	SrcInode          uint32   `json:"src_inode,omitempty" field:"source_inode"`
	SrcPathnameKey    uint32   `json:"-" field:"-"`
	SrcMountID        int32    `json:"src_mount_id,omitempty" field:"source_mount_id"`
	TargetInode       uint32   `json:"target_inode,omitempty" field:"target_inode"`
	TargetPathnameKey uint32   `json:"-" field:"-"`
	TargetMountID     int32    `json:"target_mount_id,omitempty" field:"target_mount_id"`
	Retval            int32    `json:"retval" field:"retval"`
	Event             uint32   `json:"-" field:"-"`
}

// GetProbeEventType - Returns the probe event type
func (der DentryEventRaw) GetProbeEventType() ProbeEventType {
	switch der.Event {
	case 0:
		return FileOpenEventType
	case 1:
		return FileMkdirEventType
	case 2:
		return FileHardLinkEventType
	case 3:
		return FileRenameEventType
	case 4:
		return FileUnlinkEventType
	case 5:
		return FileRmdirEventType
	case 6:
		return FileModifyEventType
	default:
		return UnknownEventType
	}
}

// GetUID - Returns the event UID
func (de *DentryEvent) GetUID() int {
	return int(de.UID)
}

// GetGID - Returns the event GID
func (de *DentryEvent) GetGID() int {
	return int(de.GID)
}

// GetMountIDs - Returns the event Mount ID
func (de *DentryEvent) GetMountIDs() []int {
	rep := []int{}
	if de.SrcMountID > 0 {
		rep = append(rep, int(de.SrcMountID))
	}
	if de.TargetMountID > 0 {
		rep = append(rep, int(de.TargetMountID))
	}
	return rep
}

// GetPid - Returns the event pid
func (de *DentryEvent) GetPid() uint32 {
	return de.Pid
}

// GetPidns - Returns the event pidns
func (de *DentryEvent) GetPidns() uint64 {
	return de.Pidns
}

// ExecveEventRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type ExecveEventRaw struct {
	Pidns         uint64   `json:"pidns"`
	Netns         uint64   `json:"netns"`
	Mntns         uint64   `json:"mntns"`
	Userns        uint64   `json:"userns"`
	Cgroup        uint64   `json:"cgroup"`
	TimestampRaw  uint64   `json:"-"`
	TTYNameRaw    [64]byte `json:"-"`
	Pid           uint32   `json:"pid"`
	Ppid          uint32   `json:"ppid"`
	Tid           uint32   `json:"tid"`
	UID           uint32   `json:"uid"`
	GID           uint32   `json:"gid"`
	Padding       uint32   `json:"-"`
	ArgKey        uint32   `json:"-"`
	PathnameKey   uint32   `json:"-"`
	PathnameInode int32    `json:"pathname_inode,omitempty"`
	MountID       int32    `json:"mount_id,omitempty"`
	Retval        int32    `json:"retval"`
	Flag          int32    `json:"flag,omitempty"`
	CommRaw       [16]byte `json:"-"`
	SigInfo       int32    `json:"sig_info,omitempty"`
	Event         uint32   `json:"-"`
}

// ExecveEvent - Execve event
type ExecveEvent struct {
	EventBase
	*ExecveEventRaw
	Argv       []string `json:"argv,omitempty"`
	BinaryPath string   `json:"binary_path,omitempty"`
	BinaryHash string   `json:"binary_hash,omitempty"`
	Comm       string   `json:"comm,omitempty"`
	TTYName    string   `json:"tty_name,omitempty"`
}

// CwdRaw - Cwd raw event (used to parse data from probe).
// (!) => members order matter
type CwdRaw struct {
	Pidns        uint64   `json:"pidns"`
	Pid          uint32   `json:"pid"`
	Tid          uint32   `json:"tid"`
	TimestampRaw uint64   `json:"-"`
	TTYNameRaw   [64]byte `json:"-"`
	PathnameKey  uint32   `json:"-"`
	Inode        uint32   `json:"inode"`
	MountID      int32    `json:"mount_id"`
}

// CwdEvent - Cwd event
type CwdEvent struct {
	EventBase
	*CwdRaw
	CurrentWorkingDirectory string `json:"current_working_directory"`
	TTYName                 string `json:"tty_name"`
}

// GetUID - Returns the event UID
func (ce *CwdEvent) GetUID() int {
	return -1
}

// GetGID - Returns the event GID
func (ce *CwdEvent) GetGID() int {
	return -1
}

// GetMountIDs - Returns the event Mount ID
func (ce *CwdEvent) GetMountIDs() []int {
	if ce.Inode > 0 {
		return []int{int(ce.MountID)}
	}
	return []int{}
}

// GetPid - Returns the event pid
func (ce *CwdEvent) GetPid() uint32 {
	return ce.Pid
}

// GetPidns - Returns the event pidns
func (ce *CwdEvent) GetPidns() uint64 {
	return ce.Pidns
}

func (ce *CwdEvent) String() string {
	return fmt.Sprintf(
		"pid:%v tid:%v pidns:%v tty:%v inode:%v mount_id:%v cwd:%v",
		ce.Pid,
		ce.Tid,
		ce.Pidns,
		ce.TTYName,
		ce.Inode,
		ce.MountID,
		ce.CurrentWorkingDirectory,
	)
}

// NamespaceSwitchRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type NamespaceSwitchRaw struct {
	OldPidns        uint64   `json:"old_pidns"`
	OldNetns        uint64   `json:"old_netns"`
	OldMntns        uint64   `json:"old_mntns"`
	OldUserns       uint64   `json:"old_userns"`
	OldCgroup       uint64   `json:"old_cgroup"`
	OldTimestampRaw uint64   `json:"-"`
	OldTTYNameRaw   [64]byte `json:"-"`
	OldPid          uint32   `json:"old_pid"`
	OldPpid         uint32   `json:"old_ppid"`
	OldTid          uint32   `json:"old_tid"`
	OldUID          uint32   `json:"old_uid"`
	OldGID          uint32   `json:"old_gid"`
	Padding1        uint32   `json:"-"`
	NewPidns        uint64   `json:"new_pidns"`
	NewNetns        uint64   `json:"new_netns"`
	NewMntns        uint64   `json:"new_mntns"`
	NewUserns       uint64   `json:"new_userns"`
	NewCgroup       uint64   `json:"new_cgroup"`
	NewTimestampRaw uint64   `json:"-"`
	NewTTYNameRaw   [64]byte `json:"-"`
	NewPid          uint32   `json:"new_pid"`
	NewPpid         uint32   `json:"new_ppid"`
	NewTid          uint32   `json:"new_tid"`
	NewUID          uint32   `json:"new_uid"`
	NewGID          uint32   `json:"new_gid"`
}

func (nsr NamespaceSwitchRaw) String() string {
	return fmt.Sprintf(
		"pid:%v->%v tid:%v->%v ppid:%v->%v pidns:%v->%v netns:%v->%v mntns:%v->%v userns:%v->%v cgroup:%v->%v uid:%v->%v gid:%v->%v",
		nsr.OldPid,
		nsr.NewPid,
		nsr.OldTid,
		nsr.NewTid,
		nsr.OldPpid,
		nsr.NewPpid,
		nsr.OldPidns,
		nsr.NewPidns,
		nsr.OldNetns,
		nsr.NewNetns,
		nsr.OldMntns,
		nsr.NewMntns,
		nsr.OldUserns,
		nsr.NewUserns,
		nsr.OldCgroup,
		nsr.NewCgroup,
		nsr.OldUID,
		nsr.NewUID,
		nsr.OldGID,
		nsr.NewGID,
	)
}

// NamespaceEnterEvent - namespace enter event
type NamespaceEnterEvent struct {
	EventBase
	*NamespaceSwitchRaw
	OldTimestamp time.Time `json:"old_timestamp"`
	NewTimestamp time.Time `json:"new_timestamp"`
	TTYName      string    `json:"tty_name"`
}

// GetUID - Returns the event UID
func (nee *NamespaceEnterEvent) GetUID() int {
	return int(nee.OldUID)
}

// GetGID - Returns the event GID
func (nee *NamespaceEnterEvent) GetGID() int {
	return int(nee.OldGID)
}

// GetMountIDs - Returns the event Mount ID
func (nee *NamespaceEnterEvent) GetMountIDs() []int {
	return []int{}
}

func (nee *NamespaceEnterEvent) String() string {
	return fmt.Sprintf("%v tty:%v", nee.NamespaceSwitchRaw, nee.TTYName)
}

// GetPid - Returns the event pid
func (nee *NamespaceEnterEvent) GetPid() uint32 {
	return nee.NewPid
}

// GetPidns - Returns the event pidns
func (nee *NamespaceEnterEvent) GetPidns() uint64 {
	return nee.NewPidns
}

// NamespaceExitEvent - namespace exit event
type NamespaceExitEvent struct {
	EventBase
	*NamespaceSwitchRaw
	OldTimestamp time.Time `json:"old_timestamp"`
	NewTimestamp time.Time `json:"new_timestamp"`
	TTYName      string    `json:"tty_name"`
}

// GetUID - Returns the event UID
func (nee *NamespaceExitEvent) GetUID() int {
	return int(nee.OldUID)
}

// GetGID - Returns the event GID
func (nee *NamespaceExitEvent) GetGID() int {
	return int(nee.OldGID)
}

// GetMountIDs - Returns the event Mount ID
func (nee *NamespaceExitEvent) GetMountIDs() []int {
	return []int{}
}

func (nee *NamespaceExitEvent) String() string {
	return fmt.Sprintf("%v tty:%v", nee.NamespaceSwitchRaw, nee.TTYName)
}

// GetPid - Returns the event pid
func (nee *NamespaceExitEvent) GetPid() uint32 {
	return nee.OldPid
}

// GetPidns - Returns the event pidns
func (nee *NamespaceExitEvent) GetPidns() uint64 {
	return nee.OldPidns
}

// SetAttrRaw - Setattr raw event (used to parse data from probe).
// (!) => members order matter
type SetAttrRaw struct {
	Pidns        uint64   `json:"pidns"`
	TimestampRaw uint64   `json:"-"`
	TTYNameRaw   [64]byte `json:"-"`
	Pid          uint32   `json:"pid"`
	Tid          uint32   `json:"tid"`
	UID          uint32   `json:"uid"`
	GID          uint32   `json:"gid"`
	Inode        uint32   `json:"inode"`
	PathnameKey  uint32   `json:"-"`
	MountID      int32    `json:"mount_id"`
	Flags        uint32   `json:"flags"`
	Mode         uint32   `json:"mode"`
	NewUID       uint32   `json:"new_uid"`
	NewGID       uint32   `json:"new_gid"`
	Padding      uint32   `json:"-"`
	AtimeRaw     [2]int64 `json:"-"`
	MtimeRaw     [2]int64 `json:"-"`
	CtimeRaw     [2]int64 `json:"-"`
	Retval       int32    `json:"retval"`
}

// SetAttrEvent - Set attr event
type SetAttrEvent struct {
	EventBase
	*SetAttrRaw
	TTYName  string    `json:"tty_name"`
	Pathname string    `json:"pathname"`
	Atime    time.Time `json:"atime"`
	Mtime    time.Time `json:"mtime"`
	Ctime    time.Time `json:"ctime"`
}

// GetUID - Returns the event UID
func (sae *SetAttrEvent) GetUID() int {
	return int(sae.UID)
}

// GetGID - Returns the event GID
func (sae *SetAttrEvent) GetGID() int {
	return int(sae.GID)
}

// GetMountIDs - Returns the event Mount ID
func (sae *SetAttrEvent) GetMountIDs() []int {
	if sae.MountID > 0 {
		return []int{int(sae.MountID)}
	}
	return []int{}
}

// GetPid - Returns the event pid
func (sae *SetAttrEvent) GetPid() uint32 {
	return sae.Pid
}

// GetPidns - Returns the event pidns
func (sae *SetAttrEvent) GetPidns() uint64 {
	return sae.Pidns
}

// SetAttrFlagsToString - Returns the string list representation of SetAttr flags
func (sae *SetAttrEvent) SetAttrFlagsToString(input uint32) []string {
	flag := SetAttrFlag(input)
	rep := []string{}
	if flag&AttrMode == AttrMode {
		rep = append(rep, "AttrMode")
	}
	if flag&AttrUID == AttrUID {
		rep = append(rep, "AttrUID")
	}
	if flag&AttrGID == AttrGID {
		rep = append(rep, "AttrGID")
	}
	if flag&AttrSize == AttrSize {
		rep = append(rep, "AttrSize")
	}
	if flag&AttrAtime == AttrAtime {
		rep = append(rep, "AttrAtime")
	}
	if flag&AttrMtime == AttrMtime {
		rep = append(rep, "AttrMtime")
	}
	if flag&AttrCtime == AttrCtime {
		rep = append(rep, "AttrCtime")
	}
	if flag&AttrAtimeSet == AttrAtimeSet {
		rep = append(rep, "AttrAtimeSet")
	}
	if flag&AttrMTimeSet == AttrMTimeSet {
		rep = append(rep, "AttrMTimeSet")
	}
	if flag&AttrForce == AttrForce {
		rep = append(rep, "AttrForce")
	}
	if flag&AttrKillSUID == AttrKillSUID {
		rep = append(rep, "AttrKillSUID")
	}
	if flag&AttrKillSGID == AttrKillSGID {
		rep = append(rep, "AttrKillSGID")
	}
	if flag&AttrFile == AttrFile {
		rep = append(rep, "AttrFile")
	}
	if flag&AttrKillPriv == AttrKillPriv {
		rep = append(rep, "AttrKillPriv")
	}
	if flag&AttrOpen == AttrOpen {
		rep = append(rep, "AttrOpen")
	}
	if flag&AttrTimesSet == AttrTimesSet {
		rep = append(rep, "AttrTimesSet")
	}
	if flag&AttrTouch == AttrTouch {
		rep = append(rep, "AttrTouch")
	}
	return rep
}

func (sae *SetAttrEvent) String() string {
	return fmt.Sprintf(
		"pid:%v tid:%v pidns:%v pathname:%v mount_id:%v inode:%v flags:%v mode:%o uid:%v gid:%v atime:%v mtime:%v ctime:%v retval:%v",
		sae.Pid,
		sae.Tid,
		sae.Pidns,
		sae.Pathname,
		sae.MountID,
		sae.Inode,
		strings.Join(sae.SetAttrFlagsToString(sae.Flags), ","),
		sae.Mode,
		sae.NewUID,
		sae.NewGID,
		sae.Atime,
		sae.Mtime,
		sae.Ctime,
		sae.Retval,
	)
}

// SocketCreateEventRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type SocketCreateEventRaw struct {
	Pidns        uint64 `json:"pidns"`
	TimestampRaw uint64 `json:"-"`
	Pid          uint32 `json:"pid"`
	Tid          uint32 `json:"tid"`
	UID          uint32 `json:"uid"`
	GID          uint32 `json:"gid"`
	Family       int32  `json:"family"`
	SocketType   int32  `json:"socket_type"`
	Protocol     int32  `json:"protocol"`
	Type         int32  `json:"type"`
	Retval       int32  `json:"retval"`
}

// SocketEvent - Socket event structure
type SocketEvent struct {
	EventBase
	*SocketCreateEventRaw
	Fd uint32 `json:"file_descriptor"`
}

// GetUID - Returns the event UID
func (se *SocketEvent) GetUID() int {
	return int(se.UID)
}

// GetGID - Returns the event GID
func (se *SocketEvent) GetGID() int {
	return int(se.GID)
}

// GetMountIDs - Returns the event Mount ID
func (se *SocketEvent) GetMountIDs() []int {
	return []int{}
}

// GetPid - Returns the event pid
func (se *SocketEvent) GetPid() uint32 {
	return se.Pid
}

// GetPidns - Returns the event pidns
func (se *SocketEvent) GetPidns() uint64 {
	return se.Pidns
}

/*
func (se *SocketEvent) String() string {
	return fmt.Sprintf(
		"SocketCreate - family:%v type:%v protocol:%v pid:%v ret:%v",
		SocketFamilyToString(se.Family),
		SocketTypeToString(se.SocketType),
		TransportProtocolToString(int64(se.Protocol)),
		se.Pid,
		ErrValueToString(se.Retval),
	)
}
*/

// SocketManipulationEventRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type SocketManipulationEventRaw struct {
	Pidns        uint64    `json:"pidns"`
	TimestampRaw uint64    `json:"-"`
	Pid          uint32    `json:"pid"`
	Tid          uint32    `json:"tid"`
	UID          uint32    `json:"uid"`
	GID          uint32    `json:"gid"`
	Fd           uint32    `json:"file_descriptor"`
	Family       uint32    `json:"family"`
	AddrRaw      [2]int64  `json:"-"`
	Port         uint64    `json:"port"`
	PathnameRaw  [108]byte `json:"-"`
	Type         int32     `json:"type"`
	Retval       int32     `json:"retval"`
	Backlog      int32     `json:"backlog"`
}

// SocketManipulationEvent - Bind event structure
type SocketManipulationEvent struct {
	EventBase
	*SocketManipulationEventRaw
	Addr     string `json:"addr"`
	Pathname string `json:"pathname"`
}

// GetUID - Returns the event UID
func (sme *SocketManipulationEvent) GetUID() int {
	return int(sme.UID)
}

// GetGID - Returns the event GID
func (sme *SocketManipulationEvent) GetGID() int {
	return int(sme.GID)
}

// GetMountIDs - Returns the event Mount ID
func (sme *SocketManipulationEvent) GetMountIDs() []int {
	return []int{}
}

// GetPid - Returns the event pid
func (sme *SocketManipulationEvent) GetPid() uint32 {
	return sme.Pid
}

// GetPidns - Returns the event pidns
func (sme *SocketManipulationEvent) GetPidns() uint64 {
	return sme.Pidns
}

/*
func (sme SocketManipulationEvent) String() string {
	addrStr := ""
	switch SocketFamily(sme.Family) {
	case AF_UNIX:
		addrStr = fmt.Sprintf("pathname:%v", sme.Pathname)
	case AF_INET:
		addrStr = fmt.Sprintf("addr:%v:%v", sme.Addr, sme.Port)
	case AF_INET6:
		addrStr = fmt.Sprintf("addr:%v port:%v", sme.Addr, sme.Port)
	}
	postfix := ""
	switch sme.EventType {
	case SocketListenEventType:
		postfix = fmt.Sprintf("backlog:%v", sme.Backlog)
	}
	return fmt.Sprintf(
		"%v - fd:%v family:%v %v pid:%v ret:%v %v",
		sme.EventType,
		sme.Fd,
		SocketFamilyToString(int32(sme.Family)),
		addrStr,
		sme.Pid,
		ErrValueToString(sme.Retval),
		postfix,
	)
}
*/

// SyscallEventRaw - Raw event definition (used to parse data from probe).
// (!) => members order matter
type SyscallEventRaw struct {
	Pidns        uint64    `json:"pidns"`
	TimestampRaw uint64    `json:"-"`
	TTYNameRaw   [64]byte  `json:"-"`
	Pid          uint32    `json:"pid"`
	Ppid         uint32    `json:"ppid"`
	Tid          uint32    `json:"tid"`
	UID          uint32    `json:"uid"`
	GID          uint32    `json:"gid"`
	Padding      uint32    `json:"-"`
	SyscallID    uint32    `json:"syscall_id"`
	RetVal       int32     `json:"retval"`
	Args         [6]uint64 `json:"args"`
	CommRaw      [16]byte  `json:"-"`
}

// SyscallEvent - Syscall event
type SyscallEvent struct {
	EventBase
	*SyscallEventRaw
	Comm    string `json:"comm"`
	TTYName string `json:"tty_name"`
}

// GetUID - Returns the event UID
func (se *SyscallEvent) GetUID() int {
	return int(se.UID)
}

// GetGID - Returns the event GID
func (se *SyscallEvent) GetGID() int {
	return int(se.GID)
}

// GetMountIDs - Returns the event Mount ID
func (se *SyscallEvent) GetMountIDs() []int {
	return []int{}
}

// GetPid - Returns the event pid
func (se *SyscallEvent) GetPid() uint32 {
	return se.Pid
}

// GetPidns - Returns the event pidns
func (se *SyscallEvent) GetPidns() uint64 {
	return se.Pidns
}

/*
func (se *SyscallEvent) String() string {
	return fmt.Sprintf(
		"syscall:%v pid:%v pidns:%v retval:%v comm:%v arg0:0x%x arg1:0x%x arg2:0x%x arg3:0x%v arg4:0x%x arg5:0x%x",
		GetSyscallName(se.SyscallID),
		se.Pid,
		se.Pidns,
		se.RetVal,
		se.Comm,
		se.Args[0],
		se.Args[1],
		se.Args[2],
		se.Args[3],
		se.Args[4],
		se.Args[5],
	)
}
*/

// ContainerEvent - Container event
type ContainerEvent struct {
	EventBase
	// Action              ContainerAction        `json:"action"`
	InitPid             uint32                 `json:"init_pid"`
	Pidns               uint64                 `json:"pidns"`
	Cgroup              uint64                 `json:"cgroup"`
	Mntns               uint64                 `json:"mntns"`
	Netns               uint64                 `json:"netns"`
	Userns              uint64                 `json:"userns"`
	Image               string                 `json:"image"`
	ContainerName       string                 `json:"container_name"`
	ContainerID         string                 `json:"container_id"`
	Digest              string                 `json:"digest"`
	Privileged          bool                   `json:"privileged"`
	CapAdd              strslice.StrSlice      `json:"cap_add"`
	AppArmorProfile     string                 `json:"apparmor_profile"`
	StartedAt           time.Time              `json:"started_at"`
	FinishedAt          time.Time              `json:"finished_at"`
	PortBindings        nat.PortMap            `json:"port_bindings"`
	SecurityOpt         []string               `json:"security_opt"`
	CommandPath         string                 `json:"command_path"`
	CommandArgs         []string               `json:"command_args"`
	OverlayFsMergedPath string                 `json:"overlayfs_merged_path"`
	Resources           container.Resources    `json:"resources"`
	NetworkSettings     *types.NetworkSettings `json:"network_settings"`
	MountPoints         []types.MountPoint     `json:"mount_points"`
}

// GetUID - Returns the event UID
func (ce *ContainerEvent) GetUID() int {
	return -1
}

// GetGID - Returns the event GID
func (ce *ContainerEvent) GetGID() int {
	return -1
}

// GetMountIDs - Returns the event Mount ID
func (ce *ContainerEvent) GetMountIDs() []int {
	return []int{}
}

// GetPid - Returns the event pid
func (ce *ContainerEvent) GetPid() uint32 {
	return 0
}

// GetPidns - Returns the event pidns
func (ce *ContainerEvent) GetPidns() uint64 {
	return ce.Pidns
}

func (ce ContainerEvent) String() string {
	networksCount := 0
	if ce.NetworkSettings != nil {
		networksCount = len(ce.NetworkSettings.Networks)
	}
	return fmt.Sprintf(
		"%v Image:%v Name:%v ContainerID:%v InitPid:%v Digest:%v Privileged:%v CapAdd:%v CommandPath:%v CommandArgs:%v NetworksCount:%v Pidns:%v Cgroup:%v Mntns:%v Netns:%v Userns:%v AppArmorProfile:%v SecurityOpt:%v",
		ce.EventType,
		ce.Image,
		ce.ContainerName,
		ce.ContainerID,
		ce.InitPid,
		ce.Digest,
		ce.Privileged,
		ce.CapAdd,
		ce.CommandPath,
		ce.CommandArgs,
		networksCount,
		ce.Pidns,
		ce.Cgroup,
		ce.Mntns,
		ce.Netns,
		ce.Userns,
		ce.AppArmorProfile,
		ce.SecurityOpt,
	)
}
