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

// RegistrySerializer serializes a registry to JSON
type RegistrySerializer struct {
	// Registry key name
	KeyName string `json:"key_name,omitempty"`
	// Registry key path
	KeyPath string `json:"key_path,omitempty"`
	// Relative name of the key
	RelativeName string `json:"key_relative_name,omitempty"`
	// Value name of the key value
	ValueName string `json:"key_value_name,omitempty"`
}

// ProcessSerializer serializes a process to JSON
type ProcessSerializer struct {
	// Process ID
	Pid uint32 `json:"pid,omitempty"`
	// Parent Process ID
	PPid *uint32 `json:"ppid,omitempty"`
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
	// User's sid
	OwnerSidString string `json:"user_sid,omitempty"`
	// User name
	User string `json:"user,omitempty"`
}

// FileEventSerializer serializes a file event to JSON
type FileEventSerializer struct {
	FileSerializer
}

// RegistryEventSerializer serializes a registry event to JSON
type RegistryEventSerializer struct {
	RegistrySerializer
}

// NetworkDeviceSerializer serializes the network device context to JSON
type NetworkDeviceSerializer struct{}

// EventSerializer serializes an event to JSON
type EventSerializer struct {
	*BaseEventSerializer
	*RegistryEventSerializer `json:"registry,omitempty"`
}

func newFileSerializer(fe *model.FileEvent, e *model.Event, _ ...uint64) *FileSerializer {
	return &FileSerializer{
		Path: e.FieldHandlers.ResolveFilePath(e, fe),
		Name: e.FieldHandlers.ResolveFileBasename(e, fe),
	}
}

func newRegistrySerializer(re *model.RegistryEvent, e *model.Event, _ ...uint64) *RegistrySerializer {
	rs := &RegistrySerializer{
		KeyName:      re.KeyName,
		KeyPath:      re.KeyPath,
		RelativeName: re.RelativeName,
		ValueName:    re.ValueName,
	}
	return rs
}
func newProcessSerializer(ps *model.Process, e *model.Event) *ProcessSerializer {
	psSerializer := &ProcessSerializer{
		ExecTime: getTimeIfNotZero(ps.ExecTime),
		ExitTime: getTimeIfNotZero(ps.ExitTime),

		Pid:            ps.Pid,
		PPid:           getUint32Pointer(&ps.PPid),
		Executable:     newFileSerializer(&ps.FileEvent, e),
		CmdLine:        e.FieldHandlers.ResolveProcessCmdLineScrubbed(e, ps),
		OwnerSidString: ps.OwnerSidString,
		User:           e.FieldHandlers.ResolveUser(e, ps),
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
	s := &EventSerializer{
		BaseEventSerializer: NewBaseEventSerializer(event),
	}
	eventType := model.EventType(event.Type)

	switch eventType {
	case model.CreateNewFileEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.CreateNewFile.File, event),
		}
		// case model.OpenEventType:
		// 	s.FileEventSerializer = &FileEventSerializer{
		// 		FileSerializer: *newFileSerializer(&event.Open.File, event),
		// 	}
	case model.CreateRegistryKeyEventType:
		s.RegistryEventSerializer = &RegistryEventSerializer{
			RegistrySerializer: *newRegistrySerializer(&event.CreateRegistryKey.Registry, event),
		}
	case model.OpenRegistryKeyEventType:
		s.RegistryEventSerializer = &RegistryEventSerializer{
			RegistrySerializer: *newRegistrySerializer(&event.OpenRegistryKey.Registry, event),
		}
	case model.SetRegistryKeyValueEventType:
		s.RegistryEventSerializer = &RegistryEventSerializer{
			RegistrySerializer: *newRegistrySerializer(&event.SetRegistryKeyValue.Registry, event),
		}
	case model.DeleteRegistryKeyEventType:
		s.RegistryEventSerializer = &RegistryEventSerializer{
			RegistrySerializer: *newRegistrySerializer(&event.DeleteRegistryKey.Registry, event),
		}
	}

	return s
}
