// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serializers holds serializers related files
package serializers

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	smodel "github.com/DataDog/datadog-agent/pkg/security/serializers/model"
)

func newAnomalyDetectionSyscallEventSerializer(e *model.AnomalyDetectionSyscallEvent) *smodel.AnomalyDetectionSyscallEventSerializer {
	return &smodel.AnomalyDetectionSyscallEventSerializer{
		Syscall: e.SyscallID.String(),
	}
}

func getInUpperLayer(f *model.FileFields) *bool {
	lowerLayer := f.GetInLowerLayer()
	upperLayer := f.GetInUpperLayer()
	if !lowerLayer && !upperLayer {
		return nil
	}
	return &upperLayer
}

func newFileSerializer(fe *model.FileEvent, e *model.Event, forceInode ...uint64) *smodel.FileSerializer {
	inode := fe.Inode
	if len(forceInode) > 0 {
		inode = forceInode[0]
	}

	mode := uint32(fe.FileFields.Mode)
	fs := &smodel.FileSerializer{
		Path:                e.FieldHandlers.ResolveFilePath(e, fe),
		PathResolutionError: fe.GetPathResolutionError(),
		Name:                e.FieldHandlers.ResolveFileBasename(e, fe),
		Inode:               getUint64Pointer(&inode),
		MountID:             getUint32Pointer(&fe.MountID),
		Filesystem:          e.FieldHandlers.ResolveFileFilesystem(e, fe),
		Mode:                getUint32Pointer(&mode), // only used by open events
		UID:                 int64(fe.UID),
		GID:                 int64(fe.GID),
		User:                e.FieldHandlers.ResolveFileFieldsUser(e, &fe.FileFields),
		Group:               e.FieldHandlers.ResolveFileFieldsGroup(e, &fe.FileFields),
		Mtime:               getTimeIfNotZero(time.Unix(0, int64(fe.MTime))),
		Ctime:               getTimeIfNotZero(time.Unix(0, int64(fe.CTime))),
		InUpperLayer:        getInUpperLayer(&fe.FileFields),
		PackageName:         e.FieldHandlers.ResolvePackageName(e, fe),
		PackageVersion:      e.FieldHandlers.ResolvePackageVersion(e, fe),
		HashState:           fe.HashState.String(),
	}

	// lazy hash serialization: we don't want to hash files for every event
	if fe.HashState == model.Done {
		fs.Hashes = e.FieldHandlers.ResolveHashesFromEvent(e, fe)
	}
	return fs
}

func newCredentialsSerializer(ce *model.Credentials) *smodel.CredentialsSerializer {
	return &smodel.CredentialsSerializer{
		UID:          int(ce.UID),
		User:         ce.User,
		EUID:         int(ce.EUID),
		EUser:        ce.EUser,
		FSUID:        int(ce.FSUID),
		FSUser:       ce.FSUser,
		GID:          int(ce.GID),
		Group:        ce.Group,
		EGID:         int(ce.EGID),
		EGroup:       ce.EGroup,
		FSGID:        int(ce.FSGID),
		FSGroup:      ce.FSGroup,
		CapEffective: model.KernelCapability(ce.CapEffective).StringArray(),
		CapPermitted: model.KernelCapability(ce.CapPermitted).StringArray(),
	}
}

func newProcessSerializer(ps *model.Process, e *model.Event, resolvers *resolvers.Resolvers) *smodel.ProcessSerializer {
	if ps.IsNotKworker() {
		argv, argvTruncated := resolvers.ProcessResolver.GetProcessArgvScrubbed(ps)
		envs, EnvsTruncated := resolvers.ProcessResolver.GetProcessEnvs(ps)
		argv0, _ := sprocess.GetProcessArgv0(ps)

		psSerializer := &smodel.ProcessSerializer{
			ForkTime: getTimeIfNotZero(ps.ForkTime),
			ExecTime: getTimeIfNotZero(ps.ExecTime),
			ExitTime: getTimeIfNotZero(ps.ExitTime),

			Pid:           ps.Pid,
			Tid:           ps.Tid,
			PPid:          getUint32Pointer(&ps.PPid),
			Comm:          ps.Comm,
			TTY:           ps.TTYName,
			Executable:    newFileSerializer(&ps.FileEvent, e),
			Argv0:         argv0,
			Args:          argv,
			ArgsTruncated: argvTruncated,
			Envs:          envs,
			EnvsTruncated: EnvsTruncated,
			IsThread:      ps.IsThread,
			IsKworker:     ps.IsKworker,
			IsExecChild:   ps.IsExecChild,
			Source:        model.ProcessSourceToString(ps.Source),
		}

		if ps.HasInterpreter() {
			psSerializer.Interpreter = newFileSerializer(&ps.LinuxBinprm.FileEvent, e)
		}

		credsSerializer := newCredentialsSerializer(&ps.Credentials)
		// Populate legacy user / group fields
		psSerializer.UID = credsSerializer.UID
		psSerializer.User = credsSerializer.User
		psSerializer.GID = credsSerializer.GID
		psSerializer.Group = credsSerializer.Group
		psSerializer.Credentials = &smodel.ProcessCredentialsSerializer{
			CredentialsSerializer: credsSerializer,
		}

		if len(ps.ContainerID) != 0 {
			psSerializer.Container = &smodel.ContainerContextSerializer{
				ID: ps.ContainerID,
			}
			if cgroup, _ := resolvers.CGroupResolver.GetWorkload(ps.ContainerID); cgroup != nil {
				psSerializer.Container.CreatedAt = getTimeIfNotZero(time.Unix(0, int64(cgroup.CreatedAt)))
			}
		}
		return psSerializer
	}
	return &smodel.ProcessSerializer{
		Pid:         ps.Pid,
		Tid:         ps.Tid,
		IsKworker:   ps.IsKworker,
		IsExecChild: ps.IsExecChild,
		Source:      model.ProcessSourceToString(ps.Source),
		Credentials: &smodel.ProcessCredentialsSerializer{
			CredentialsSerializer: &smodel.CredentialsSerializer{},
		},
	}
}

func newUserContextSerializer(e *model.Event) *smodel.UserContextSerializer {
	return &smodel.UserContextSerializer{
		User:  e.ProcessContext.User,
		Group: e.ProcessContext.Group,
	}
}

func newSELinuxSerializer(e *model.Event) *smodel.SELinuxEventSerializer {
	switch e.SELinux.EventKind {
	case model.SELinuxBoolChangeEventKind:
		return &smodel.SELinuxEventSerializer{
			BoolChange: &smodel.SELinuxBoolChangeSerializer{
				Name:  e.FieldHandlers.ResolveSELinuxBoolName(e, &e.SELinux),
				State: e.SELinux.BoolChangeValue,
			},
		}
	case model.SELinuxStatusChangeEventKind:
		return &smodel.SELinuxEventSerializer{
			EnforceStatus: &smodel.SELinuxEnforceStatusSerializer{
				Status: e.SELinux.EnforceStatus,
			},
		}
	case model.SELinuxBoolCommitEventKind:
		return &smodel.SELinuxEventSerializer{
			BoolCommit: &smodel.SELinuxBoolCommitSerializer{
				State: e.SELinux.BoolCommitValue,
			},
		}
	default:
		return nil
	}
}

func newBPFMapSerializer(e *model.Event) *smodel.BPFMapSerializer {
	if e.BPF.Map.ID == 0 {
		return nil
	}
	return &smodel.BPFMapSerializer{
		Name:    e.BPF.Map.Name,
		MapType: model.BPFMapType(e.BPF.Map.Type).String(),
	}
}

func newBPFProgramSerializer(e *model.Event) *smodel.BPFProgramSerializer {
	if e.BPF.Program.ID == 0 {
		return nil
	}

	return &smodel.BPFProgramSerializer{
		Name:        e.BPF.Program.Name,
		Tag:         e.BPF.Program.Tag,
		ProgramType: model.BPFProgramType(e.BPF.Program.Type).String(),
		AttachType:  model.BPFAttachType(e.BPF.Program.AttachType).String(),
		Helpers:     model.StringifyHelpersList(e.BPF.Program.Helpers),
	}
}

func newBPFEventSerializer(e *model.Event) *smodel.BPFEventSerializer {
	return &smodel.BPFEventSerializer{
		Cmd:     model.BPFCmd(e.BPF.Cmd).String(),
		Map:     newBPFMapSerializer(e),
		Program: newBPFProgramSerializer(e),
	}
}

func newMMapEventSerializer(e *model.Event) *smodel.MMapEventSerializer {
	return &smodel.MMapEventSerializer{
		Address:    fmt.Sprintf("0x%x", e.MMap.Addr),
		Offset:     e.MMap.Offset,
		Len:        e.MMap.Len,
		Protection: model.Protection(e.MMap.Protection).String(),
		Flags:      model.MMapFlag(e.MMap.Flags).String(),
	}
}

func newMProtectEventSerializer(e *model.Event) *smodel.MProtectEventSerializer {
	return &smodel.MProtectEventSerializer{
		VMStart:       fmt.Sprintf("0x%x", e.MProtect.VMStart),
		VMEnd:         fmt.Sprintf("0x%x", e.MProtect.VMEnd),
		VMProtection:  model.VMFlag(e.MProtect.VMProtection).String(),
		ReqProtection: model.VMFlag(e.MProtect.ReqProtection).String(),
	}
}

func newPTraceEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *smodel.PTraceEventSerializer {
	return &smodel.PTraceEventSerializer{
		Request: model.PTraceRequest(e.PTrace.Request).String(),
		Address: fmt.Sprintf("0x%x", e.PTrace.Address),
		Tracee:  newProcessContextSerializer(e.PTrace.Tracee, e, resolvers),
	}
}

func newLoadModuleEventSerializer(e *model.Event) *smodel.ModuleEventSerializer {
	loadedFromMemory := e.LoadModule.LoadedFromMemory
	argsTruncated := e.LoadModule.ArgsTruncated
	return &smodel.ModuleEventSerializer{
		Name:             e.LoadModule.Name,
		LoadedFromMemory: &loadedFromMemory,
		Argv:             e.FieldHandlers.ResolveModuleArgv(e, &e.LoadModule),
		ArgsTruncated:    &argsTruncated,
	}
}

func newUnloadModuleEventSerializer(e *model.Event) *smodel.ModuleEventSerializer {
	return &smodel.ModuleEventSerializer{
		Name: e.UnloadModule.Name,
	}
}

func newSignalEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *smodel.SignalEventSerializer {
	ses := &smodel.SignalEventSerializer{
		Type:   model.Signal(e.Signal.Type).String(),
		PID:    e.Signal.PID,
		Target: newProcessContextSerializer(e.Signal.Target, e, resolvers),
	}
	return ses
}

func newSpliceEventSerializer(e *model.Event) *smodel.SpliceEventSerializer {
	return &smodel.SpliceEventSerializer{
		PipeEntryFlag: model.PipeBufFlag(e.Splice.PipeEntryFlag).String(),
		PipeExitFlag:  model.PipeBufFlag(e.Splice.PipeExitFlag).String(),
	}
}

func newBindEventSerializer(e *model.Event) *smodel.BindEventSerializer {
	bes := &smodel.BindEventSerializer{
		Addr: newIPPortFamilySerializer(&e.Bind.Addr,
			model.AddressFamily(e.Bind.AddrFamily).String()),
	}
	return bes
}

func newMountEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *smodel.MountEventSerializer {
	fh := e.FieldHandlers

	src, srcErr := resolvers.PathResolver.ResolveMountRoot(e, &e.Mount.Mount)
	dst, dstErr := resolvers.PathResolver.ResolveMountPoint(e, &e.Mount.Mount)
	mountPointPath := fh.ResolveMountPointPath(e, &e.Mount)
	mountSourcePath := fh.ResolveMountSourcePath(e, &e.Mount)

	mountSerializer := &smodel.MountEventSerializer{
		MountPoint: &smodel.FileSerializer{
			Path:    dst,
			MountID: &e.Mount.ParentPathKey.MountID,
			Inode:   &e.Mount.ParentPathKey.Inode,
		},
		Root: &smodel.FileSerializer{
			Path:    src,
			MountID: &e.Mount.RootPathKey.MountID,
			Inode:   &e.Mount.RootPathKey.Inode,
		},
		MountID:         e.Mount.MountID,
		ParentMountID:   e.Mount.ParentPathKey.MountID,
		BindSrcMountID:  e.Mount.BindSrcMountID,
		Device:          e.Mount.Device,
		FSType:          e.Mount.GetFSType(),
		MountPointPath:  mountPointPath,
		MountSourcePath: mountSourcePath,
	}

	if srcErr != nil {
		mountSerializer.Root.PathResolutionError = srcErr.Error()
	}
	if dstErr != nil {
		mountSerializer.MountPoint.PathResolutionError = dstErr.Error()
	}
	// potential errors retrieved from ResolveMountPointPath and ResolveMountSourcePath
	if e.Mount.MountPointPathResolutionError != nil {
		mountSerializer.MountPointPathResolutionError = e.Mount.MountPointPathResolutionError.Error()
	}
	if e.Mount.MountSourcePathResolutionError != nil {
		mountSerializer.MountSourcePathResolutionError = e.Mount.MountSourcePathResolutionError.Error()
	}

	return mountSerializer
}

func newNetworkDeviceSerializer(e *model.Event) *smodel.NetworkDeviceSerializer {
	return &smodel.NetworkDeviceSerializer{
		NetNS:   e.NetworkContext.Device.NetNS,
		IfIndex: e.NetworkContext.Device.IfIndex,
		IfName:  e.FieldHandlers.ResolveNetworkDeviceIfName(e, &e.NetworkContext.Device),
	}
}

func serializeOutcome(retval int64) string {
	switch {
	case retval < 0:
		if syscall.Errno(-retval) == syscall.EACCES || syscall.Errno(-retval) == syscall.EPERM {
			return "Refused"
		}
		return "Error"
	default:
		return "Success"
	}
}

func newProcessContextSerializer(pc *model.ProcessContext, e *model.Event, resolvers *resolvers.Resolvers) *smodel.ProcessContextSerializer {
	if pc == nil || pc.Pid == 0 || e == nil {
		return nil
	}

	ps := smodel.ProcessContextSerializer{
		ProcessSerializer: newProcessSerializer(&pc.Process, e, resolvers),
	}

	ctx := eval.NewContext(e)

	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	var ancestor *model.ProcessCacheEntry
	var prev *smodel.ProcessSerializer

	first := true

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)

		s := newProcessSerializer(&pce.Process, e, resolvers)
		ps.Ancestors = append(ps.Ancestors, s)

		if first {
			ps.Parent = s
		}
		first = false

		// dedup args/envs
		if ancestor != nil && ancestor.ArgsEntry == pce.ArgsEntry {
			prev.Args, prev.ArgsTruncated = prev.Args[0:0], false
			prev.Envs, prev.EnvsTruncated = prev.Envs[0:0], false
			prev.Argv0 = ""
		}
		ancestor = pce
		prev = s

		ptr = it.Next()
	}

	return &ps
}

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *model.Event, resolvers *resolvers.Resolvers) *smodel.EventSerializer {
	s := &smodel.EventSerializer{
		BaseEventSerializer:   NewBaseEventSerializer(event, resolvers),
		UserContextSerializer: newUserContextSerializer(event),
	}
	s.Async = event.FieldHandlers.ResolveAsync(event)

	if id := event.FieldHandlers.ResolveContainerID(event, event.ContainerContext); id != "" {
		var creationTime time.Time
		if cgroup, _ := resolvers.CGroupResolver.GetWorkload(id); cgroup != nil {
			creationTime = time.Unix(0, int64(cgroup.CreatedAt))
		}
		s.ContainerContextSerializer = &smodel.ContainerContextSerializer{
			ID:        id,
			CreatedAt: getTimeIfNotZero(creationTime),
		}
	}

	eventType := model.EventType(event.Type)

	switch eventType {
	case model.FileChmodEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chmod.File, event),
			Destination: &smodel.FileSerializer{
				Mode: &event.Chmod.Mode,
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Chmod.Retval)
	case model.FileChownEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chown.File, event),
			Destination: &smodel.FileSerializer{
				UID: event.Chown.UID,
				GID: event.Chown.GID,
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Chown.Retval)
	case model.FileLinkEventType:
		// use the source inode as the target one is a fake inode
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Link.Source, event),
			Destination:    newFileSerializer(&event.Link.Target, event, event.Link.Source.Inode),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Link.Retval)
	case model.FileOpenEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Open.File, event),
		}

		if event.Open.Flags&syscall.O_CREAT > 0 {
			s.FileEventSerializer.Destination = &smodel.FileSerializer{
				Mode: &event.Open.Mode,
			}
		}

		s.FileSerializer.Flags = model.OpenFlags(event.Open.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeOutcome(event.Open.Retval)
	case model.FileMkdirEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Mkdir.File, event),
			Destination: &smodel.FileSerializer{
				Mode: &event.Mkdir.Mode,
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Mkdir.Retval)
	case model.FileRmdirEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rmdir.File, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Rmdir.Retval)
	case model.FileUnlinkEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Unlink.File, event),
		}
		s.FileSerializer.Flags = model.UnlinkFlags(event.Unlink.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeOutcome(event.Unlink.Retval)
	case model.FileRenameEventType:
		// use the new inode as the old one is a fake inode
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rename.Old, event, event.Rename.New.Inode),
			Destination:    newFileSerializer(&event.Rename.New, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Rename.Retval)
	case model.FileRemoveXAttrEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.RemoveXAttr.File, event),
			Destination: &smodel.FileSerializer{
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.RemoveXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.RemoveXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.RemoveXAttr.Retval)
	case model.FileSetXAttrEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SetXAttr.File, event),
			Destination: &smodel.FileSerializer{
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.SetXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.SetXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.SetXAttr.Retval)
	case model.FileUtimesEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Utimes.File, event),
			Destination: &smodel.FileSerializer{
				Atime: getTimeIfNotZero(event.Utimes.Atime),
				Mtime: getTimeIfNotZero(event.Utimes.Mtime),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Utimes.Retval)
	case model.FileMountEventType:
		s.MountEventSerializer = newMountEventSerializer(event, resolvers)
		s.EventContextSerializer.Outcome = serializeOutcome(event.Mount.Retval)
	case model.FileUmountEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			NewMountID: event.Umount.MountID,
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Umount.Retval)
	case model.SetuidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &smodel.SetuidSerializer{
			UID:    int(event.SetUID.UID),
			User:   event.FieldHandlers.ResolveSetuidUser(event, &event.SetUID),
			EUID:   int(event.SetUID.EUID),
			EUser:  event.FieldHandlers.ResolveSetuidEUser(event, &event.SetUID),
			FSUID:  int(event.SetUID.FSUID),
			FSUser: event.FieldHandlers.ResolveSetuidFSUser(event, &event.SetUID),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.SetgidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &smodel.SetgidSerializer{
			GID:     int(event.SetGID.GID),
			Group:   event.FieldHandlers.ResolveSetgidGroup(event, &event.SetGID),
			EGID:    int(event.SetGID.EGID),
			EGroup:  event.FieldHandlers.ResolveSetgidEGroup(event, &event.SetGID),
			FSGID:   int(event.SetGID.FSGID),
			FSGroup: event.FieldHandlers.ResolveSetgidFSGroup(event, &event.SetGID),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.CapsetEventType:
		s.ProcessContextSerializer.Credentials.Destination = &smodel.CapsetSerializer{
			CapEffective: model.KernelCapability(event.Capset.CapEffective).StringArray(),
			CapPermitted: model.KernelCapability(event.Capset.CapPermitted).StringArray(),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.ForkEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.SELinuxEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SELinux.File, event),
		}
		s.SELinuxEventSerializer = newSELinuxSerializer(event)
	case model.BPFEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.BPFEventSerializer = newBPFEventSerializer(event)
	case model.MMapEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.MMap.Retval)
		if event.MMap.Flags&unix.MAP_ANONYMOUS == 0 {
			s.FileEventSerializer = &smodel.FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.MMap.File, event),
			}
		}
		s.MMapEventSerializer = newMMapEventSerializer(event)
	case model.MProtectEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.MProtect.Retval)
		s.MProtectEventSerializer = newMProtectEventSerializer(event)
	case model.PTraceEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.PTrace.Retval)
		s.PTraceEventSerializer = newPTraceEventSerializer(event, resolvers)
	case model.LoadModuleEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.LoadModule.Retval)
		if !event.LoadModule.LoadedFromMemory {
			s.FileEventSerializer = &smodel.FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.LoadModule.File, event),
			}
		}
		s.ModuleEventSerializer = newLoadModuleEventSerializer(event)
	case model.UnloadModuleEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.UnloadModule.Retval)
		s.ModuleEventSerializer = newUnloadModuleEventSerializer(event)
	case model.SignalEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.Signal.Retval)
		s.SignalEventSerializer = newSignalEventSerializer(event, resolvers)
	case model.SpliceEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.Splice.Retval)
		s.SpliceEventSerializer = newSpliceEventSerializer(event)
		if event.Splice.File.Inode != 0 {
			s.FileEventSerializer = &smodel.FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.Splice.File, event),
			}
		}
	case model.BindEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.Bind.Retval)
		s.BindEventSerializer = newBindEventSerializer(event)
	case model.AnomalyDetectionSyscallEventType:
		s.AnomalyDetectionSyscallEventSerializer = newAnomalyDetectionSyscallEventSerializer(&event.AnomalyDetectionSyscallEvent)
	case model.DNSEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.DNSEventSerializer = newDNSEventSerializer(&event.DNS)
	}

	return s
}
