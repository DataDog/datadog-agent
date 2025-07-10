// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

// yamlEvent represents an event that can be marshaled to and unmarshaled from
// YAML.
type yamlEvent struct {
	event event
}

// wrapEventForYAML wraps an event for YAML marshaling
func wrapEventForYAML(ev event) yamlEvent {
	return yamlEvent{event: ev}
}

// MarshalYAML implements custom YAML marshaling for events.
func (ye yamlEvent) MarshalYAML() (interface{}, error) {
	encodeNodeTag := func(tag string, data interface{}) (*yaml.Node, error) {
		node := &yaml.Node{}
		err := node.Encode(data)
		if err != nil {
			return nil, err
		}
		node.Tag = tag
		return node, nil
	}

	switch ev := ye.event.(type) {
	case eventProcessesUpdated:
		type processUpdateYaml struct {
			ProcessID struct {
				PID int `yaml:"pid"`
			} `yaml:"process_id"`
			Executable Executable       `yaml:"executable"`
			Probes     []map[string]any `yaml:"probes"`
		}

		eventData := struct {
			TenantID tenantID            `yaml:"tenant_id,omitempty"`
			Updated  []processUpdateYaml `yaml:"updated,omitempty"`
			Removed  []int               `yaml:"removed,omitempty"`
		}{
			TenantID: ev.tenantID,
		}

		// Convert updated processes
		for _, proc := range ev.updated {
			var probes []map[string]any
			for _, probe := range proc.Probes {
				probeJSON, err := json.Marshal(probe)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal probe: %w", err)
				}
				var probeData map[string]any
				if err := json.Unmarshal(probeJSON, &probeData); err != nil {
					return nil, fmt.Errorf("failed to unmarshal probe: %w", err)
				}
				probes = append(probes, probeData)
			}

			eventData.Updated = append(eventData.Updated, processUpdateYaml{
				ProcessID: struct {
					PID int `yaml:"pid"`
				}{PID: int(proc.ProcessID.PID)},
				Executable: proc.Executable,
				Probes:     probes,
			})
		}

		// Convert removed IDs
		for _, id := range ev.removed {
			eventData.Removed = append(eventData.Removed, int(id.PID))
		}

		return encodeNodeTag("!processes-updated", eventData)

	case eventProgramLoaded:
		return encodeNodeTag("!loaded", map[string]int{
			"program_id": int(ev.programID),
		})

	case eventProgramLoadingFailed:
		return encodeNodeTag("!loading-failed", map[string]any{
			"program_id": int(ev.programID),
			"error":      ev.err.Error(),
		})

	case eventProgramAttached:
		return encodeNodeTag("!attached", map[string]int{
			"program_id": int(ev.program.ir.ID),
			"process_id": int(ev.program.procID.PID),
		})

	case eventProgramAttachingFailed:
		return encodeNodeTag("!attaching-failed", map[string]any{
			"program_id": int(ev.programID),
			"process_id": int(ev.processID.PID),
			"error":      ev.err.Error(),
		})

	case eventProgramDetached:
		return encodeNodeTag("!detached", map[string]int{
			"program_id": int(ev.programID),
			"process_id": int(ev.processID.PID),
		})

	case eventShutdown:
		return encodeNodeTag("!shutdown", map[string]any{})

	default:
		return nil, fmt.Errorf("unknown event type: %T", ev)
	}
}

// UnmarshalYAML implements custom YAML unmarshaling for events
func (ye *yamlEvent) UnmarshalYAML(node *yaml.Node) error {
	// Use the YAML tag to determine the event type
	eventType := strings.TrimPrefix(node.Tag, "!")

	switch eventType {
	case "processes-updated":
		type processUpdateYaml struct {
			ProcessID struct {
				PID int `yaml:"pid"`
			} `yaml:"process_id"`
			Executable struct {
				Path string `yaml:"path"`
				Key  struct {
					FileHandle struct {
						Dev uint64 `yaml:"dev"`
						Ino uint64 `yaml:"ino"`
					} `yaml:"file_handle"`
					FileCookie struct {
						Sec  int64 `yaml:"sec"`
						Nsec int64 `yaml:"nsec"`
					} `yaml:"file_cookie"`
				} `yaml:"key"`
			} `yaml:"executable"`
			Probes []map[string]any `yaml:"probes"`
		}

		var eventData struct {
			TenantID tenantID            `yaml:"tenant_id,omitempty"`
			Updated  []processUpdateYaml `yaml:"updated,omitempty"`
			Removed  []int               `yaml:"removed,omitempty"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode processes-updated event: %w", err)
		}

		// Convert updated processes
		var updated []ProcessUpdate
		for _, proc := range eventData.Updated {
			var probes []ir.ProbeDefinition
			for _, p := range proc.Probes {
				probeJSON, err := json.Marshal(p)
				if err != nil {
					return fmt.Errorf("failed to marshal probe: %w", err)
				}
				rcProbe, err := rcjson.UnmarshalProbe(probeJSON)
				if err != nil {
					return fmt.Errorf("failed to unmarshal probe: %w", err)
				}
				probes = append(probes, rcProbe)
			}

			updated = append(updated, ProcessUpdate{
				ProcessID: ProcessID{PID: int32(proc.ProcessID.PID)},
				Executable: Executable{
					Path: proc.Executable.Path,
					Key: procmon.FileKey{
						FileHandle: procmon.FileHandle{
							Dev: proc.Executable.Key.FileHandle.Dev,
							Ino: proc.Executable.Key.FileHandle.Ino,
						},
						LastModified: syscall.Timespec{
							Sec:  proc.Executable.Key.FileCookie.Sec,
							Nsec: proc.Executable.Key.FileCookie.Nsec,
						},
					},
				},
				Probes: probes,
			})
		}

		// Convert removed IDs to ProcessIDs
		var removedProcessIDs []ProcessID
		for _, id := range eventData.Removed {
			removedProcessIDs = append(removedProcessIDs, ProcessID{PID: int32(id)})
		}

		ye.event = eventProcessesUpdated{
			tenantID: eventData.TenantID,
			updated:  updated,
			removed:  removedProcessIDs,
		}

	case "loaded":
		var eventData struct {
			ProgramID int `yaml:"program_id"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode loaded event: %w", err)
		}
		ye.event = eventProgramLoaded{
			programID: ir.ProgramID(eventData.ProgramID),
			loaded: &loadedProgram{
				ir: &ir.Program{ID: ir.ProgramID(eventData.ProgramID)},
			},
		}

	case "loading-failed":
		var eventData struct {
			ProgramID int    `yaml:"program_id"`
			Error     string `yaml:"error"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode loading-failed event: %w", err)
		}
		ye.event = eventProgramLoadingFailed{
			programID: ir.ProgramID(eventData.ProgramID),
			err:       errors.New(eventData.Error),
		}

	case "attached":
		var eventData struct {
			ProgramID int `yaml:"program_id"`
			ProcessID int `yaml:"process_id"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode attached event: %w", err)
		}
		ye.event = eventProgramAttached{
			program: &attachedProgram{
				ir:     &ir.Program{ID: ir.ProgramID(eventData.ProgramID)},
				procID: ProcessID{PID: int32(eventData.ProcessID)},
			},
		}

	case "attaching-failed":
		var eventData struct {
			ProgramID int    `yaml:"program_id"`
			ProcessID int    `yaml:"process_id"`
			Error     string `yaml:"error"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode attaching-failed event: %w", err)
		}
		ye.event = eventProgramAttachingFailed{
			programID: ir.ProgramID(eventData.ProgramID),
			processID: ProcessID{PID: int32(eventData.ProcessID)},
			err:       errors.New(eventData.Error),
		}

	case "detached":
		var eventData struct {
			ProgramID int `yaml:"program_id"`
			ProcessID int `yaml:"process_id"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode detached event: %w", err)
		}
		ye.event = eventProgramDetached{
			programID: ir.ProgramID(eventData.ProgramID),
			processID: ProcessID{PID: int32(eventData.ProcessID)},
		}

	case "shutdown":
		ye.event = eventShutdown{}

	default:
		return fmt.Errorf("unknown event type: %s", eventType)
	}

	return nil
}
