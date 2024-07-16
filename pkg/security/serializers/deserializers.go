// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package serializers

import (
	json "encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func getPointerValue[T uint32 | uint64 | bool](p *T) T {
	if p != nil {
		return *p
	}
	var def T
	return def
}

func newFileEvent(fs *FileSerializer) model.FileEvent {
	file := model.FileEvent{
		PathnameStr: fs.Path,
		BasenameStr: fs.Name,
		Filesystem:  fs.Filesystem,
		FileFields: model.FileFields{
			UID:          uint32(fs.UID),
			User:         fs.User,
			GID:          uint32(fs.GID),
			Group:        fs.Group,
			InUpperLayer: getPointerValue(fs.InUpperLayer),
			Mode:         uint16(getPointerValue(fs.Mode)),
			MTime:        uint64(fs.Mtime.GetInnerTime().UnixMicro()),
			CTime:        uint64(fs.Ctime.GetInnerTime().UnixMicro()),
			PathKey: model.PathKey{
				Inode:   getPointerValue(fs.Inode),
				MountID: getPointerValue(fs.MountID),
			},
		},
		PkgName:               fs.PackageName,
		PkgVersion:            fs.PackageVersion,
		IsPathnameStrResolved: true,
		IsBasenameStrResolved: true,
		HashState:             model.NoHash,
	}
	return file
}

func newProcess(ps *ProcessSerializer) model.Process {
	p := model.Process{
		PPid:          getPointerValue(ps.PPid),
		Comm:          ps.Comm,
		TTYName:       ps.TTY,
		FileEvent:     newFileEvent(ps.Executable),
		Argv0:         ps.Argv0,
		Argv:          ps.Args,
		ArgsTruncated: ps.ArgsTruncated,
		Envs:          ps.Envs,
		EnvsTruncated: ps.EnvsTruncated,
		IsThread:      ps.IsThread,
		IsExecExec:    ps.IsExecExec,
		PIDContext: model.PIDContext{
			Pid:       ps.Pid,
			Tid:       ps.Tid,
			IsKworker: ps.IsKworker,
		},
	}
	if ps.ForkTime != nil {
		p.ForkTime = ps.ForkTime.GetInnerTime()
	}
	if ps.ExecTime != nil {
		p.ExecTime = ps.ExecTime.GetInnerTime()
	}
	if ps.ExitTime != nil {
		p.ExitTime = ps.ExitTime.GetInnerTime()
	}
	if ps.Container != nil {
		p.ContainerID = model.ContainerID(ps.Container.ID)
	}

	// TODO: credentials
	return p
}

// UnmarshalEvent unmarshal an model.Event (only exec one for now)
func UnmarshalEvent(raw []byte) (*model.Event, error) {
	rawEvent := EventSerializer{}
	err := json.Unmarshal(raw, &rawEvent)
	if err != nil {
		return nil, err
	}
	if rawEvent.EventContextSerializer.Name != "exec" {
		return nil, fmt.Errorf("Unmarshalling of event %v is not yet supported", rawEvent.EventContextSerializer.Name)
	}

	parent := newProcess(rawEvent.ProcessContextSerializer.Parent)
	process := newProcess(rawEvent.ProcessContextSerializer.ProcessSerializer)
	event := model.Event{
		BaseEvent: model.BaseEvent{
			Type:             uint32(model.ExecEventType),
			FieldHandlers:    &model.FakeFieldHandlers{},
			ContainerContext: &model.ContainerContext{},
			ProcessContext: &model.ProcessContext{
				Process:  process,
				Parent:   &parent,
				Ancestor: nil,
			},
		},
	}
	event.BaseEvent.ProcessCacheEntry = &model.ProcessCacheEntry{
		ProcessContext: *event.BaseEvent.ProcessContext,
	}

	// Fill ancestors
	prevProcessContext := event.BaseEvent.ProcessContext
	prevProcess := &process
	for _, ancestor := range rawEvent.ProcessContextSerializer.Ancestors {
		currentPocess := newProcess(ancestor)
		prevProcessContext.Ancestor = &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{
				Parent:  prevProcess,
				Process: currentPocess,
			},
		}
		prevProcessContext = &prevProcessContext.Ancestor.ProcessContext
		prevProcess = &currentPocess
	}

	event.BaseEvent.ProcessCacheEntry = &model.ProcessCacheEntry{
		ProcessContext: *event.BaseEvent.ProcessContext,
	}

	return &event, nil
}

// DecodeEvent will read a JSON file, and unmarshal its content to an model.Event
func DecodeEvent(file string) (*model.Event, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return UnmarshalEvent(raw)
}
