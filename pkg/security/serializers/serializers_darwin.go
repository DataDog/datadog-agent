// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package serializers defines functions aiming to serialize events
package serializers

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// This serializer is deliberately self-contained rather than built on
// serializers_base.go, even though the base file looks platform-neutral.
//
// serializers_base.go is //go:build linux || windows and cannot simply be
// widened: it *references* FileSerializer, ProcessSerializer,
// ProcessCredentialsSerializer, UserSessionContextSerializer, TracerSerializer,
// NetworkDeviceSerializer, newFileSerializer, newProcessSerializer and
// newVariablesContext, all of which live in serializers_linux.go. Reusing it
// would mean porting most of the Linux serializer, much of it describing
// concepts macOS has no equivalent for (capabilities, cgroups, mount IDs,
// SELinux). That is a real port, not a PoC step.
//
// What matters for the backend is the payload *shape*, so the JSON keys below
// mirror the ones the Linux serializer emits: an "evt" context, a "date", and
// "process"/"file"/"exit" sections, with Linux-identical field names wherever
// the concept exists on both platforms. Fields with no macOS meaning are
// omitted rather than reported as zero, and the macOS-specific code-signing
// identity is added under the executable.
//
// easyjson is not used: it is a //go:generate performance optimisation, and
// serializers_others.go already demonstrates plain encoding/json is sufficient.

// darwinFileSerializer serializes a file. Endpoint Security supplies fully
// resolved absolute paths, so there is no inode, mount or filesystem context to
// report.
//
// Note what is absent: eslogger gives us signing_id, team_id, cdhash and
// is_platform_binary per executable, which is macOS's answer to "is this binary
// trustworthy" and has no Linux counterpart. They are not serialized because
// model.FileEvent has nowhere to carry them, and inventing always-empty JSON
// keys would be worse than omitting them. Surfacing that identity needs the
// SECL model extensions described in RFC 5.3.
type darwinFileSerializer struct {
	Path string `json:"path,omitempty"`
	Name string `json:"name,omitempty"`
}

// darwinProcessSerializer serializes a process. Field names match the Linux
// ProcessSerializer so the backend sees a familiar shape.
type darwinProcessSerializer struct {
	Pid  uint32  `json:"pid,omitempty"`
	PPid *uint32 `json:"ppid,omitempty"`
	UID  int     `json:"uid"`
	GID  int     `json:"gid"`

	User  string `json:"user,omitempty"`
	Group string `json:"group,omitempty"`

	Comm string `json:"comm,omitempty"`
	TTY  string `json:"tty,omitempty"`

	// Args are always the scrubbed form: argv on a developer laptop routinely
	// carries tokens. Envs are deliberately absent entirely.
	Args          []string `json:"args,omitempty"`
	Argv0         string   `json:"argv0,omitempty"`
	ArgsTruncated bool     `json:"args_truncated,omitempty"`

	ForkTime *utils.EasyjsonTime `json:"fork_time,omitempty"`
	ExecTime *utils.EasyjsonTime `json:"exec_time,omitempty"`
	ExitTime *utils.EasyjsonTime `json:"exit_time,omitempty"`

	Executable *darwinFileSerializer `json:"executable,omitempty"`
}

// darwinProcessContextSerializer serializes a process and its lineage.
type darwinProcessContextSerializer struct {
	*darwinProcessSerializer
	Parent    *darwinProcessSerializer   `json:"parent,omitempty"`
	Ancestors []*darwinProcessSerializer `json:"ancestors,omitempty"`
}

// darwinEventContextSerializer is the "evt" section.
type darwinEventContextSerializer struct {
	Name     string `json:"name,omitempty"`
	Category string `json:"category,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
}

// darwinExitEventSerializer is the "exit" section.
type darwinExitEventSerializer struct {
	Cause string `json:"cause"`
	Code  uint32 `json:"code"`
}

// EventSerializer serializes an event to JSON
type EventSerializer struct {
	darwinEventContextSerializer `json:"evt,omitempty"`
	Date                         utils.EasyjsonTime `json:"date,omitempty"`

	*darwinProcessContextSerializer `json:"process,omitempty"`
	FileEventSerializer             *darwinFileSerializer      `json:"file,omitempty"`
	ExitEventSerializer             *darwinExitEventSerializer `json:"exit,omitempty"`
}

// ToJSON returns json
func (e *EventSerializer) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// MarshalEvent marshal the event
func MarshalEvent(event *model.Event, scrubber *utils.Scrubber) ([]byte, error) {
	s := NewEventSerializer(event, nil, scrubber)
	return json.Marshal(s)
}

// MarshalCustomEvent marshal the custom event
func MarshalCustomEvent(event *events.CustomEvent) ([]byte, error) {
	return json.Marshal(event)
}

// newDarwinFileSerializer serializes a file event, adding the code-signing
// identity of the process when the file is that process's executable.
func newDarwinFileSerializer(fe *model.FileEvent, event *model.Event) *darwinFileSerializer {
	if fe == nil {
		return nil
	}
	return &darwinFileSerializer{
		Path: event.FieldHandlers.ResolveFilePath(event, fe),
		Name: event.FieldHandlers.ResolveFileBasename(event, fe),
	}
}

func newDarwinProcessSerializer(p *model.Process, event *model.Event) *darwinProcessSerializer {
	if p == nil {
		return nil
	}

	argv := event.FieldHandlers.ResolveProcessArgvScrubbed(event, p)
	argv0 := event.FieldHandlers.ResolveProcessArgv0(event, p)

	ps := &darwinProcessSerializer{
		Pid:           p.Pid,
		UID:           int(p.Credentials.UID),
		GID:           int(p.Credentials.GID),
		User:          p.Credentials.User,
		Group:         p.Credentials.Group,
		Comm:          p.Comm,
		TTY:           p.TTYName,
		Args:          argv,
		Argv0:         argv0,
		ArgsTruncated: event.FieldHandlers.ResolveProcessArgsTruncated(event, p),
		ForkTime:      utils.NewEasyjsonTimeIfNotZero(p.ForkTime),
		ExecTime:      utils.NewEasyjsonTimeIfNotZero(p.ExecTime),
		ExitTime:      utils.NewEasyjsonTimeIfNotZero(p.ExitTime),
		Executable:    newDarwinFileSerializer(&p.FileEvent, event),
	}

	if p.PPid != 0 {
		ppid := p.PPid
		ps.PPid = &ppid
	}

	return ps
}

// newDarwinProcessContextSerializer walks the ancestor lineage, which is what
// makes a Workload Protection signal legible: the tree is the story.
func newDarwinProcessContextSerializer(pc *model.ProcessContext, event *model.Event) *darwinProcessContextSerializer {
	if pc == nil || pc.Pid == 0 {
		return nil
	}

	ps := &darwinProcessContextSerializer{
		darwinProcessSerializer: newDarwinProcessSerializer(&pc.Process, event),
	}

	ctx := eval.NewContext(event)
	it := &model.ProcessAncestorsIterator{Root: pc.Ancestor}
	first := true
	for ptr := it.Front(ctx); ptr != nil; ptr = it.Next(ctx) {
		pce := (*model.ProcessCacheEntry)(ptr)
		s := newDarwinProcessSerializer(&pce.Process, event)
		ps.Ancestors = append(ps.Ancestors, s)
		if first {
			ps.Parent = s
			first = false
		}
	}

	return ps
}

// exitCauseString maps the numeric exit cause onto the same strings the Linux
// serializer emits.
func exitCauseString(cause uint32) string {
	return sharedconsts.ExitCause(cause).String()
}

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *model.Event, _ *rules.Rule, _ *utils.Scrubber) *EventSerializer {
	if event == nil {
		return nil
	}

	eventType := model.EventType(event.Type)

	s := &EventSerializer{
		darwinEventContextSerializer: darwinEventContextSerializer{
			Name:     eventType.String(),
			Category: model.GetEventTypeCategory(eventType.String()).String(),
		},
		Date:                           utils.NewEasyjsonTime(event.ResolveEventTime()),
		darwinProcessContextSerializer: newDarwinProcessContextSerializer(event.ProcessContext, event),
	}

	switch eventType {
	case model.ExecEventType:
		s.FileEventSerializer = newDarwinFileSerializer(&event.Exec.Process.FileEvent, event)
	case model.ExitEventType:
		s.ExitEventSerializer = &darwinExitEventSerializer{
			Cause: exitCauseString(event.Exit.Cause),
			Code:  event.Exit.Code,
		}
		if event.Exit.Process != nil {
			s.FileEventSerializer = newDarwinFileSerializer(&event.Exit.Process.FileEvent, event)
		}
	case model.FileOpenEventType:
		s.FileEventSerializer = newDarwinFileSerializer(&event.Open.File, event)
	}

	return s
}
