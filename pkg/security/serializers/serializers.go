//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/probe/doc_generator -output ../../../docs/cloud-workload-security/backend.schema.json

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package serializers

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/serializers/jsonmodel"
	jwriter "github.com/mailru/easyjson/jwriter"
)

func getInUpperLayer(f *model.FileFields) *bool {
	lowerLayer := f.GetInLowerLayer()
	upperLayer := f.GetInUpperLayer()
	if !lowerLayer && !upperLayer {
		return nil
	}
	return &upperLayer
}

func newFileSerializer(fe *model.FileEvent, e *model.Event, forceInode ...uint64) *jsonmodel.FileSerializer {
	inode := fe.Inode
	if len(forceInode) > 0 {
		inode = forceInode[0]
	}

	mode := uint32(fe.FileFields.Mode)
	return &jsonmodel.FileSerializer{
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
	}
}

func getUint64Pointer(i *uint64) *uint64 {
	if *i == 0 {
		return nil
	}
	return i
}

func getUint32Pointer(i *uint32) *uint32 {
	if *i == 0 {
		return nil
	}
	return i
}

func getTimeIfNotZero(t time.Time) *jsonmodel.EasyjsonTime {
	if t.IsZero() {
		return nil
	}
	tt := jsonmodel.NewEasyjsonTime(t)
	return &tt
}

func newCredentialsSerializer(ce *model.Credentials) *jsonmodel.CredentialsSerializer {
	return &jsonmodel.CredentialsSerializer{
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

func newProcessSerializer(ps *model.Process, e *model.Event, resolvers *resolvers.Resolvers) *jsonmodel.ProcessSerializer {
	if ps.IsNotKworker() {
		argv, argvTruncated := resolvers.ProcessResolver.GetProcessScrubbedArgv(ps)
		envs, EnvsTruncated := resolvers.ProcessResolver.GetProcessEnvs(ps)
		argv0, _ := resolvers.ProcessResolver.GetProcessArgv0(ps)

		psSerializer := &jsonmodel.ProcessSerializer{
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
		psSerializer.Credentials = &jsonmodel.ProcessCredentialsSerializer{
			CredentialsSerializer: credsSerializer,
		}

		if len(ps.ContainerID) != 0 {
			psSerializer.Container = &jsonmodel.ContainerContextSerializer{
				ID: ps.ContainerID,
			}
			if cgroup, _ := resolvers.CGroupResolver.GetWorkload(ps.ContainerID); cgroup != nil {
				psSerializer.Container.CreatedAt = getTimeIfNotZero(time.Unix(0, int64(cgroup.CreationTime)))
			}
		}
		return psSerializer
	} else {
		return &jsonmodel.ProcessSerializer{
			Pid:         ps.Pid,
			Tid:         ps.Tid,
			IsKworker:   ps.IsKworker,
			Credentials: &jsonmodel.ProcessCredentialsSerializer{},
		}
	}
}

func newDDContextSerializer(e *model.Event) *jsonmodel.DDContextSerializer {
	s := &jsonmodel.DDContextSerializer{
		SpanID:  e.SpanContext.SpanID,
		TraceID: e.SpanContext.TraceID,
	}
	if s.SpanID != 0 || s.TraceID != 0 {
		return s
	}

	ctx := eval.NewContext(e)
	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)

		if pce.SpanID != 0 || pce.TraceID != 0 {
			s.SpanID = pce.SpanID
			s.TraceID = pce.TraceID
			break
		}

		ptr = it.Next()
	}

	return s
}

func newUserContextSerializer(e *model.Event) *jsonmodel.UserContextSerializer {
	return &jsonmodel.UserContextSerializer{
		User:  e.ProcessContext.User,
		Group: e.ProcessContext.Group,
	}
}

func newProcessContextSerializer(pc *model.ProcessContext, e *model.Event, resolvers *resolvers.Resolvers) *jsonmodel.ProcessContextSerializer {
	if pc == nil || pc.Pid == 0 || e == nil {
		return nil
	}

	lastPid := pc.Pid

	ps := jsonmodel.ProcessContextSerializer{
		ProcessSerializer: newProcessSerializer(&pc.Process, e, resolvers),
	}

	ctx := eval.NewContext(e)

	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	var ancestor *model.ProcessCacheEntry
	var prev *jsonmodel.ProcessSerializer

	first := true

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)
		lastPid = pce.Pid

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

	if lastPid != 1 {
		resolvers.ProcessResolver.CountBrokenLineage()
	}

	return &ps
}

func newSELinuxSerializer(e *model.Event) *jsonmodel.SELinuxEventSerializer {
	switch e.SELinux.EventKind {
	case model.SELinuxBoolChangeEventKind:
		return &jsonmodel.SELinuxEventSerializer{
			BoolChange: &jsonmodel.SELinuxBoolChangeSerializer{
				Name:  e.FieldHandlers.ResolveSELinuxBoolName(e, &e.SELinux),
				State: e.SELinux.BoolChangeValue,
			},
		}
	case model.SELinuxStatusChangeEventKind:
		return &jsonmodel.SELinuxEventSerializer{
			EnforceStatus: &jsonmodel.SELinuxEnforceStatusSerializer{
				Status: e.SELinux.EnforceStatus,
			},
		}
	case model.SELinuxBoolCommitEventKind:
		return &jsonmodel.SELinuxEventSerializer{
			BoolCommit: &jsonmodel.SELinuxBoolCommitSerializer{
				State: e.SELinux.BoolCommitValue,
			},
		}
	default:
		return nil
	}
}

func newBPFMapSerializer(e *model.Event) *jsonmodel.BPFMapSerializer {
	if e.BPF.Map.ID == 0 {
		return nil
	}
	return &jsonmodel.BPFMapSerializer{
		Name:    e.BPF.Map.Name,
		MapType: model.BPFMapType(e.BPF.Map.Type).String(),
	}
}

func newBPFProgramSerializer(e *model.Event) *jsonmodel.BPFProgramSerializer {
	if e.BPF.Program.ID == 0 {
		return nil
	}

	return &jsonmodel.BPFProgramSerializer{
		Name:        e.BPF.Program.Name,
		Tag:         e.BPF.Program.Tag,
		ProgramType: model.BPFProgramType(e.BPF.Program.Type).String(),
		AttachType:  model.BPFAttachType(e.BPF.Program.AttachType).String(),
		Helpers:     model.StringifyHelpersList(e.BPF.Program.Helpers),
	}
}

func newBPFEventSerializer(e *model.Event) *jsonmodel.BPFEventSerializer {
	return &jsonmodel.BPFEventSerializer{
		Cmd:     model.BPFCmd(e.BPF.Cmd).String(),
		Map:     newBPFMapSerializer(e),
		Program: newBPFProgramSerializer(e),
	}
}

func newMMapEventSerializer(e *model.Event) *jsonmodel.MMapEventSerializer {
	return &jsonmodel.MMapEventSerializer{
		Address:    fmt.Sprintf("0x%x", e.MMap.Addr),
		Offset:     e.MMap.Offset,
		Len:        e.MMap.Len,
		Protection: model.Protection(e.MMap.Protection).String(),
		Flags:      model.MMapFlag(e.MMap.Flags).String(),
	}
}

func newMProtectEventSerializer(e *model.Event) *jsonmodel.MProtectEventSerializer {
	return &jsonmodel.MProtectEventSerializer{
		VMStart:       fmt.Sprintf("0x%x", e.MProtect.VMStart),
		VMEnd:         fmt.Sprintf("0x%x", e.MProtect.VMEnd),
		VMProtection:  model.VMFlag(e.MProtect.VMProtection).String(),
		ReqProtection: model.VMFlag(e.MProtect.ReqProtection).String(),
	}
}

func newPTraceEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *jsonmodel.PTraceEventSerializer {
	return &jsonmodel.PTraceEventSerializer{
		Request: model.PTraceRequest(e.PTrace.Request).String(),
		Address: fmt.Sprintf("0x%x", e.PTrace.Address),
		Tracee:  newProcessContextSerializer(e.PTrace.Tracee, e, resolvers),
	}
}

func newLoadModuleEventSerializer(e *model.Event) *jsonmodel.ModuleEventSerializer {
	loadedFromMemory := e.LoadModule.LoadedFromMemory
	return &jsonmodel.ModuleEventSerializer{
		Name:             e.LoadModule.Name,
		LoadedFromMemory: &loadedFromMemory,
	}
}

func newUnloadModuleEventSerializer(e *model.Event) *jsonmodel.ModuleEventSerializer {
	return &jsonmodel.ModuleEventSerializer{
		Name: e.UnloadModule.Name,
	}
}

func newSignalEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *jsonmodel.SignalEventSerializer {
	ses := &jsonmodel.SignalEventSerializer{
		Type:   model.Signal(e.Signal.Type).String(),
		PID:    e.Signal.PID,
		Target: newProcessContextSerializer(e.Signal.Target, e, resolvers),
	}
	return ses
}

func newSpliceEventSerializer(e *model.Event) *jsonmodel.SpliceEventSerializer {
	return &jsonmodel.SpliceEventSerializer{
		PipeEntryFlag: model.PipeBufFlag(e.Splice.PipeEntryFlag).String(),
		PipeExitFlag:  model.PipeBufFlag(e.Splice.PipeExitFlag).String(),
	}
}

func newDNSQuestionSerializer(d *model.DNSEvent) *jsonmodel.DNSQuestionSerializer {
	return &jsonmodel.DNSQuestionSerializer{
		Class: model.QClass(d.Class).String(),
		Type:  model.QType(d.Type).String(),
		Name:  d.Name,
		Size:  d.Size,
		Count: d.Count,
	}
}

func newDNSEventSerializer(d *model.DNSEvent) *jsonmodel.DNSEventSerializer {
	return &jsonmodel.DNSEventSerializer{
		ID:       d.ID,
		Question: newDNSQuestionSerializer(d),
	}
}

func newIPPortSerializer(c *model.IPPortContext) *jsonmodel.IPPortSerializer {
	return &jsonmodel.IPPortSerializer{
		IP:   c.IPNet.IP.String(),
		Port: c.Port,
	}
}

func newIPPortFamilySerializer(c *model.IPPortContext, family string) *jsonmodel.IPPortFamilySerializer {
	return &jsonmodel.IPPortFamilySerializer{
		IP:     c.IPNet.IP.String(),
		Port:   c.Port,
		Family: family,
	}
}

func newNetworkDeviceSerializer(e *model.Event) *jsonmodel.NetworkDeviceSerializer {
	return &jsonmodel.NetworkDeviceSerializer{
		NetNS:   e.NetworkContext.Device.NetNS,
		IfIndex: e.NetworkContext.Device.IfIndex,
		IfName:  e.FieldHandlers.ResolveNetworkDeviceIfName(e, &e.NetworkContext.Device),
	}
}

func newNetworkContextSerializer(e *model.Event) *jsonmodel.NetworkContextSerializer {
	return &jsonmodel.NetworkContextSerializer{
		Device:      newNetworkDeviceSerializer(e),
		L3Protocol:  model.L3Protocol(e.NetworkContext.L3Protocol).String(),
		L4Protocol:  model.L4Protocol(e.NetworkContext.L4Protocol).String(),
		Source:      newIPPortSerializer(&e.NetworkContext.Source),
		Destination: newIPPortSerializer(&e.NetworkContext.Destination),
		Size:        e.NetworkContext.Size,
	}
}

func newBindEventSerializer(e *model.Event) *jsonmodel.BindEventSerializer {
	bes := &jsonmodel.BindEventSerializer{
		Addr: newIPPortFamilySerializer(&e.Bind.Addr,
			model.AddressFamily(e.Bind.AddrFamily).String()),
	}
	return bes
}

func newExitEventSerializer(e *model.Event) *jsonmodel.ExitEventSerializer {
	return &jsonmodel.ExitEventSerializer{
		Cause: model.ExitCause(e.Exit.Cause).String(),
		Code:  e.Exit.Code,
	}
}

func newMountEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *jsonmodel.MountEventSerializer {
	fh := e.FieldHandlers

	src, srcErr := resolvers.PathResolver.ResolveMountRoot(e, &e.Mount.Mount)
	dst, dstErr := resolvers.PathResolver.ResolveMountPoint(e, &e.Mount.Mount)
	mountPointPath := fh.ResolveMountPointPath(e, &e.Mount)
	mountSourcePath := fh.ResolveMountSourcePath(e, &e.Mount)

	mountSerializer := &jsonmodel.MountEventSerializer{
		MountPoint: &jsonmodel.FileSerializer{
			Path:    dst,
			MountID: &e.Mount.ParentMountID,
			Inode:   &e.Mount.ParentInode,
		},
		Root: &jsonmodel.FileSerializer{
			Path:    src,
			MountID: &e.Mount.RootMountID,
			Inode:   &e.Mount.RootInode,
		},
		MountID:         e.Mount.MountID,
		GroupID:         e.Mount.GroupID,
		ParentMountID:   e.Mount.ParentMountID,
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

func serializeSyscallRetval(retval int64) string {
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

func MarshalEvent(event *model.Event, probe *resolvers.Resolvers) ([]byte, error) {
	s := NewEventSerializer(event, probe)
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	s.MarshalEasyJSON(w)
	return w.BuildBytes()
}

func MarshalCustomEvent(event *events.CustomEvent) ([]byte, error) {
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	event.MarshalEasyJSON(w)
	return w.BuildBytes()
}

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *model.Event, resolvers *resolvers.Resolvers) *jsonmodel.EventSerializer {
	var pc model.ProcessContext
	if entry, _ := event.FieldHandlers.ResolveProcessCacheEntry(event); entry != nil {
		pc = entry.ProcessContext
	}

	s := &jsonmodel.EventSerializer{
		EventContextSerializer: jsonmodel.EventContextSerializer{
			Name:  model.EventType(event.Type).String(),
			Async: event.FieldHandlers.ResolveAsync(event),
		},
		ProcessContextSerializer: newProcessContextSerializer(&pc, event, resolvers),
		DDContextSerializer:      newDDContextSerializer(event),
		UserContextSerializer:    newUserContextSerializer(event),
		Date:                     jsonmodel.NewEasyjsonTime(event.FieldHandlers.ResolveEventTimestamp(event)),
	}

	if id := event.FieldHandlers.ResolveContainerID(event, &event.ContainerContext); id != "" {
		var creationTime time.Time
		if cgroup, _ := resolvers.CGroupResolver.GetWorkload(id); cgroup != nil {
			creationTime = time.Unix(0, int64(cgroup.CreationTime))
		}
		s.ContainerContextSerializer = &jsonmodel.ContainerContextSerializer{
			ID:        id,
			CreatedAt: getTimeIfNotZero(creationTime),
		}
	}

	eventType := model.EventType(event.Type)

	s.Category = model.GetEventTypeCategory(eventType.String())

	if s.Category == model.NetworkCategory {
		s.NetworkContextSerializer = newNetworkContextSerializer(event)
	}

	switch eventType {
	case model.FileChmodEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chmod.File, event),
			Destination: &jsonmodel.FileSerializer{
				Mode: &event.Chmod.Mode,
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Chmod.Retval)
	case model.FileChownEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chown.File, event),
			Destination: &jsonmodel.FileSerializer{
				UID: event.Chown.UID,
				GID: event.Chown.GID,
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Chown.Retval)
	case model.FileLinkEventType:
		// use the source inode as the target one is a fake inode
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Link.Source, event),
			Destination:    newFileSerializer(&event.Link.Target, event, event.Link.Source.Inode),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Link.Retval)
	case model.FileOpenEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Open.File, event),
		}

		if event.Open.Flags&syscall.O_CREAT > 0 {
			s.FileEventSerializer.Destination = &jsonmodel.FileSerializer{
				Mode: &event.Open.Mode,
			}
		}

		s.FileSerializer.Flags = model.OpenFlags(event.Open.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Open.Retval)
	case model.FileMkdirEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Mkdir.File, event),
			Destination: &jsonmodel.FileSerializer{
				Mode: &event.Mkdir.Mode,
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mkdir.Retval)
	case model.FileRmdirEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rmdir.File, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rmdir.Retval)
	case model.FileUnlinkEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Unlink.File, event),
		}
		s.FileSerializer.Flags = model.UnlinkFlags(event.Unlink.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Unlink.Retval)
	case model.FileRenameEventType:
		// use the new inode as the old one is a fake inode
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rename.Old, event, event.Rename.New.Inode),
			Destination:    newFileSerializer(&event.Rename.New, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rename.Retval)
	case model.FileRemoveXAttrEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.RemoveXAttr.File, event),
			Destination: &jsonmodel.FileSerializer{
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.RemoveXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.RemoveXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.RemoveXAttr.Retval)
	case model.FileSetXAttrEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SetXAttr.File, event),
			Destination: &jsonmodel.FileSerializer{
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.SetXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.SetXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.SetXAttr.Retval)
	case model.FileUtimesEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Utimes.File, event),
			Destination: &jsonmodel.FileSerializer{
				Atime: getTimeIfNotZero(event.Utimes.Atime),
				Mtime: getTimeIfNotZero(event.Utimes.Mtime),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Utimes.Retval)
	case model.FileMountEventType:
		s.MountEventSerializer = newMountEventSerializer(event, resolvers)
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mount.Retval)
	case model.FileUmountEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			NewMountID: event.Umount.MountID,
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Umount.Retval)
	case model.SetuidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &jsonmodel.SetuidSerializer{
			UID:    int(event.SetUID.UID),
			User:   event.FieldHandlers.ResolveSetuidUser(event, &event.SetUID),
			EUID:   int(event.SetUID.EUID),
			EUser:  event.FieldHandlers.ResolveSetuidEUser(event, &event.SetUID),
			FSUID:  int(event.SetUID.FSUID),
			FSUser: event.FieldHandlers.ResolveSetuidFSUser(event, &event.SetUID),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.SetgidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &jsonmodel.SetgidSerializer{
			GID:     int(event.SetGID.GID),
			Group:   event.FieldHandlers.ResolveSetgidGroup(event, &event.SetGID),
			EGID:    int(event.SetGID.EGID),
			EGroup:  event.FieldHandlers.ResolveSetgidEGroup(event, &event.SetGID),
			FSGID:   int(event.SetGID.FSGID),
			FSGroup: event.FieldHandlers.ResolveSetgidFSGroup(event, &event.SetGID),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.CapsetEventType:
		s.ProcessContextSerializer.Credentials.Destination = &jsonmodel.CapsetSerializer{
			CapEffective: model.KernelCapability(event.Capset.CapEffective).StringArray(),
			CapPermitted: model.KernelCapability(event.Capset.CapPermitted).StringArray(),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.ForkEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.ExitEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.ExitEventSerializer = newExitEventSerializer(event)
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.ExecEventType:
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.SELinuxEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.FileEventSerializer = &jsonmodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SELinux.File, event),
		}
		s.SELinuxEventSerializer = newSELinuxSerializer(event)
	case model.BPFEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.BPFEventSerializer = newBPFEventSerializer(event)
	case model.MMapEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.MMap.Retval)
		if event.MMap.Flags&unix.MAP_ANONYMOUS == 0 {
			s.FileEventSerializer = &jsonmodel.FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.MMap.File, event),
			}
		}
		s.MMapEventSerializer = newMMapEventSerializer(event)
	case model.MProtectEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.MProtect.Retval)
		s.MProtectEventSerializer = newMProtectEventSerializer(event)
	case model.PTraceEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.PTrace.Retval)
		s.PTraceEventSerializer = newPTraceEventSerializer(event, resolvers)
	case model.LoadModuleEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.LoadModule.Retval)
		if !event.LoadModule.LoadedFromMemory {
			s.FileEventSerializer = &jsonmodel.FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.LoadModule.File, event),
			}
		}
		s.ModuleEventSerializer = newLoadModuleEventSerializer(event)
	case model.UnloadModuleEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.UnloadModule.Retval)
		s.ModuleEventSerializer = newUnloadModuleEventSerializer(event)
	case model.SignalEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Signal.Retval)
		s.SignalEventSerializer = newSignalEventSerializer(event, resolvers)
	case model.SpliceEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Splice.Retval)
		s.SpliceEventSerializer = newSpliceEventSerializer(event)
		if event.Splice.File.Inode != 0 {
			s.FileEventSerializer = &jsonmodel.FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.Splice.File, event),
			}
		}
	case model.DNSEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.DNSEventSerializer = newDNSEventSerializer(&event.DNS)
	case model.BindEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Bind.Retval)
		s.BindEventSerializer = newBindEventSerializer(event)
	}

	return s
}
