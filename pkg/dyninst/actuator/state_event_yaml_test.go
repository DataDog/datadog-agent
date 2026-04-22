// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	procinfo "github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

// eventConfig is a pseudo-event used in snapshot tests to configure state
// machine parameters (e.g. discoveredTypesLimit) before processing real events.
type eventConfig struct {
	baseEvent
	discoveredTypesLimit   int
	recompilationRateLimit float64
	recompilationRateBurst int
}

func (e eventConfig) String() string {
	return fmt.Sprintf("eventConfig{discoveredTypesLimit: %d, recompilationRateLimit: %g, recompilationRateBurst: %d}",
		e.discoveredTypesLimit, e.recompilationRateLimit, e.recompilationRateBurst)
}

// yamlEvent represents an event that can be marshaled to and unmarshaled from
// YAML.
type yamlEvent struct {
	event event
}

type eventRuntimeStatsUpdated struct {
	baseEvent
	programID    ir.ProgramID
	runtimeStats []loader.RuntimeStats
}

func (e eventRuntimeStatsUpdated) String() string {
	return fmt.Sprintf(
		"eventRuntimeStatsUpdated{programID: %v}",
		e.programID,
	)
}

type runtimeStatsUpdater interface {
	setRuntimeStats([]loader.RuntimeStats)
}

// wrapEventForYAML wraps an event for YAML marshaling
func wrapEventForYAML(ev event) yamlEvent {
	return yamlEvent{event: ev}
}

// MarshalYAML implements custom YAML marshaling for events.
func (ye yamlEvent) MarshalYAML() (rv any, err error) {
	encodeNodeTag := func(tag string, data any) (*yaml.Node, error) {
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
			Service    string           `yaml:"service,omitempty"`
			Probes     []map[string]any `yaml:"probes"`
		}

		eventData := struct {
			Updated []processUpdateYaml `yaml:"updated,omitempty"`
			Removed []int               `yaml:"removed,omitempty"`
		}{}

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
				Service:    proc.Info.Service,
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
		})

	case eventProgramAttached:
		return encodeNodeTag("!attached", map[string]int{
			"program_id": int(ev.program.programID),
			"process_id": int(ev.program.processID.PID),
		})

	case eventProgramAttachingFailed:
		return encodeNodeTag("!attaching-failed", map[string]any{
			"program_id": int(ev.programID),
			"process_id": int(ev.processID.PID),
		})

	case eventProgramDetached:
		return encodeNodeTag("!detached", map[string]int{
			"program_id": int(ev.programID),
			"process_id": int(ev.processID.PID),
		})

	case eventProgramUnloaded:
		return encodeNodeTag("!unloaded", map[string]int{
			"program_id": int(ev.programID),
		})

	case eventHeartbeatCheck:
		return encodeNodeTag("!heartbeat-check", map[string]any{})

	case eventRuntimeStatsUpdated:
		return encodeNodeTag("!runtime-stats", map[string]any{
			"program_id":    int(ev.programID),
			"runtime_stats": runtimeStatsToYAML(ev.runtimeStats),
		})

	case eventMissingTypesReported:
		return encodeNodeTag("!missing-types-reported", map[string]any{
			"process_id": int(ev.processID.PID),
			"type_names": ev.typeNames,
		})

	case eventShutdown:
		return encodeNodeTag("!shutdown", map[string]any{})

	case eventConfig:
		data := map[string]any{
			"discovered_types_limit": ev.discoveredTypesLimit,
		}
		if ev.recompilationRateLimit != 0 {
			data["recompilation_rate_limit"] = ev.recompilationRateLimit
		}
		if ev.recompilationRateBurst != 0 {
			data["recompilation_rate_burst"] = ev.recompilationRateBurst
		}
		return encodeNodeTag("!config", data)

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
			Service string           `yaml:"service,omitempty"`
			Probes  []map[string]any `yaml:"probes"`
		}

		var eventData struct {
			Updated []processUpdateYaml `yaml:"updated,omitempty"`
			Removed []int               `yaml:"removed,omitempty"`
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
				Info: procinfo.Info{
					ProcessID: ProcessID{PID: int32(proc.ProcessID.PID)},
					Service:   proc.Service,
					Executable: Executable{
						Path: proc.Executable.Path,
						Key: procinfo.FileKey{
							FileHandle: procinfo.FileHandle{
								Dev: proc.Executable.Key.FileHandle.Dev,
								Ino: proc.Executable.Key.FileHandle.Ino,
							},
							LastModified: syscall.Timespec{
								Sec:  proc.Executable.Key.FileCookie.Sec,
								Nsec: proc.Executable.Key.FileCookie.Nsec,
							},
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
			updated: updated,
			removed: removedProcessIDs,
		}

	case "loaded":
		var eventData struct {
			ProgramID    int                `yaml:"program_id"`
			RuntimeStats []runtimeStatsYAML `yaml:"runtime_stats,omitempty"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode loaded event: %w", err)
		}
		ye.event = eventProgramLoaded{
			programID: ir.ProgramID(eventData.ProgramID),
			loaded: &loadedProgram{
				programID: ir.ProgramID(eventData.ProgramID),
				loaded: &fakeLoadedProgram{
					runtimeStats: runtimeStatsFromYAML(
						eventData.RuntimeStats,
					),
				},
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
				loadedProgram: &loadedProgram{
					programID: ir.ProgramID(eventData.ProgramID),
					loaded:    &fakeLoadedProgram{},
				},
				processID: ProcessID{PID: int32(eventData.ProcessID)},
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

	case "unloaded":
		var eventData struct {
			ProgramID int `yaml:"program_id"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode unloaded event: %w", err)
		}
		ye.event = eventProgramUnloaded{
			programID: ir.ProgramID(eventData.ProgramID),
		}

	case "heartbeat-check":
		ye.event = eventHeartbeatCheck{}

	case "runtime-stats":
		var eventData struct {
			ProgramID    int                `yaml:"program_id"`
			RuntimeStats []runtimeStatsYAML `yaml:"runtime_stats,omitempty"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode runtime-stats event: %w", err)
		}
		ye.event = eventRuntimeStatsUpdated{
			programID: ir.ProgramID(eventData.ProgramID),
			runtimeStats: runtimeStatsFromYAML(
				eventData.RuntimeStats,
			),
		}

	case "missing-types-reported":
		var eventData struct {
			ProcessID int      `yaml:"process_id"`
			TypeNames []string `yaml:"type_names"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode missing-types-reported event: %w", err)
		}
		ye.event = eventMissingTypesReported{
			processID: ProcessID{PID: int32(eventData.ProcessID)},
			typeNames: eventData.TypeNames,
		}

	case "shutdown":
		ye.event = eventShutdown{}

	case "config":
		var eventData struct {
			DiscoveredTypesLimit   int     `yaml:"discovered_types_limit"`
			RecompilationRateLimit float64 `yaml:"recompilation_rate_limit"`
			RecompilationRateBurst int     `yaml:"recompilation_rate_burst"`
		}
		if err := node.Decode(&eventData); err != nil {
			return fmt.Errorf("failed to decode config event: %w", err)
		}
		ye.event = eventConfig{
			discoveredTypesLimit:   eventData.DiscoveredTypesLimit,
			recompilationRateLimit: eventData.RecompilationRateLimit,
			recompilationRateBurst: eventData.RecompilationRateBurst,
		}

	default:
		return fmt.Errorf("unknown event type: %s", eventType)
	}

	return nil
}

type runtimeStatsYAML struct {
	CPU          time.Duration `yaml:"cpu"`
	HitCnt       uint64        `yaml:"hit_cnt"`
	ThrottledCnt uint64        `yaml:"throttled_cnt"`
}

type fakeLoadedProgram struct {
	runtimeStats []loader.RuntimeStats
}

func (*fakeLoadedProgram) Attach(ProcessID, Executable) (AttachedProgram, error) {
	return nil, nil
}

func (p *fakeLoadedProgram) RuntimeStats() []loader.RuntimeStats {
	if len(p.runtimeStats) > 0 {
		return p.runtimeStats
	}
	return []loader.RuntimeStats{
		{
			HitCnt:       1000,
			ThrottledCnt: 999,
			CPU:          1e3 * time.Second,
		},
	}
}

func (p *fakeLoadedProgram) setRuntimeStats(stats []loader.RuntimeStats) {
	p.runtimeStats = append([]loader.RuntimeStats(nil), stats...)
}

func (*fakeLoadedProgram) Close() error {
	return nil
}

var _ LoadedProgram = (*fakeLoadedProgram)(nil)

func runtimeStatsFromYAML(
	stats []runtimeStatsYAML,
) []loader.RuntimeStats {
	if len(stats) == 0 {
		return nil
	}
	converted := make([]loader.RuntimeStats, len(stats))
	for i, stat := range stats {
		converted[i] = loader.RuntimeStats{
			CPU:          stat.CPU,
			HitCnt:       stat.HitCnt,
			ThrottledCnt: stat.ThrottledCnt,
		}
	}
	return converted
}

func runtimeStatsToYAML(
	stats []loader.RuntimeStats,
) []runtimeStatsYAML {
	if len(stats) == 0 {
		return nil
	}
	converted := make([]runtimeStatsYAML, len(stats))
	for i, stat := range stats {
		converted[i] = runtimeStatsYAML{
			CPU:          stat.CPU,
			HitCnt:       stat.HitCnt,
			ThrottledCnt: stat.ThrottledCnt,
		}
	}
	return converted
}
