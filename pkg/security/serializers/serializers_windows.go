// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializers

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// FileSerializer serializes a file to JSON
// easyjson:json
type FileSerializer struct {
	// File path
	Path string `json:"path,omitempty"`
	// File basename
	Name string `json:"name,omitempty"`
}

// ProcessSerializer serializes a process to JSON
type ProcessSerializer struct {
	// Process ID
	Pid uint32 `json:"pid,omitempty"`
	// Parent Process ID
	PPid *uint32 `json:"ppid,omitempty"`
	// User of Process
	User string `json:"user,omitempty"`
	// Exec time of the process
	ExecTime *utils.EasyjsonTime `json:"exec_time,omitempty"`
	// Exit time of the process
	ExitTime *utils.EasyjsonTime `json:"exit_time,omitempty"`
	// File information of the executable
	Executable *FileSerializer `json:"executable,omitempty"`
	// Container context
	Container *ContainerContextSerializer `json:"container,omitempty"`
	// Command line arguments
	CmdLine string `json:"cmdline,omitempty"`
}

// FileEventSerializer serializes a file event to JSON
type FileEventSerializer struct {
	FileSerializer
}

// NetworkDeviceSerializer serializes the network device context to JSON
type NetworkDeviceSerializer struct{}

// EventSerializer serializes an event to JSON
type EventSerializer struct {
	*BaseEventSerializer
}

func newFileSerializer(fe *model.FileEvent, e *model.Event, _ ...uint64) *FileSerializer {
	return &FileSerializer{
		Path: e.FieldHandlers.ResolveFilePath(e, fe),
		Name: e.FieldHandlers.ResolveFileBasename(e, fe),
	}
}

func newProcessSerializer(ps *model.Process, e *model.Event) *ProcessSerializer {
	psSerializer := &ProcessSerializer{
		ExecTime: getTimeIfNotZero(ps.ExecTime),
		ExitTime: getTimeIfNotZero(ps.ExitTime),

		Pid:        ps.Pid,
		PPid:       getUint32Pointer(&ps.PPid),
		User:       ps.User,
		Executable: newFileSerializer(&ps.FileEvent, e),
		CmdLine:    e.FieldHandlers.ResolveProcessCmdLineScrubbed(e, ps),
	}

	if len(ps.ContainerID) != 0 {
		psSerializer.Container = &ContainerContextSerializer{
			ID: ps.ContainerID,
		}
	}
	return psSerializer
}

func newProcessContextSerializer(pc *model.ProcessContext, e *model.Event) *ProcessContextSerializer {
	if pc == nil || pc.Pid == 0 || e == nil {
		return nil
	}

	ps := ProcessContextSerializer{
		ProcessSerializer: newProcessSerializer(&pc.Process, e),
	}

	ctx := eval.NewContext(e)

	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	first := true

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)

		s := newProcessSerializer(&pce.Process, e)
		ps.Ancestors = append(ps.Ancestors, s)

		if first {
			ps.Parent = s
		}
		first = false

		ptr = it.Next()
	}

	return &ps
}

func serializeOutcome(_ int64) string {
	return "unknown"
}

// ToJSON returns json
func (e *EventSerializer) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// MarshalEvent marshal the event
func MarshalEvent(event *model.Event) ([]byte, error) {
	s := NewEventSerializer(event)
	return json.Marshal(s)
}

// MarshalCustomEvent marshal the custom event
func MarshalCustomEvent(event *events.CustomEvent) ([]byte, error) {
	return json.Marshal(event)
}

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *model.Event) *EventSerializer {
	return &EventSerializer{
		BaseEventSerializer: NewBaseEventSerializer(event),
	}
}
