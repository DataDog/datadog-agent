// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procsubscribe

import (
	"maps"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe/procscan"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type effects interface {
	clearPendingRequests()
	track(runtimeID string)
	untrack(runtimeID string)
	emitUpdate(update process.ProcessesUpdate)
}

type subscriberState struct {
	streamEstablished bool
	tracked           map[string]*runtimeEntry
	pidToRuntime      map[int32]string
}

type runtimeEntry struct {
	process.Info
	runtimeID    string
	probesByPath map[string]ir.ProbeDefinition
	symdbEnabled bool
}

func makeSubscriberState() subscriberState {
	return subscriberState{
		tracked:      make(map[string]*runtimeEntry),
		pidToRuntime: make(map[int32]string),
	}
}

func (s *subscriberState) onStreamEstablished(effects effects) {
	s.streamEstablished = true
	toTrack := make([]string, 0, len(s.tracked))
	for _, entry := range s.tracked {
		toTrack = append(toTrack, entry.runtimeID)
	}
	slices.Sort(toTrack)
	for _, runtimeID := range toTrack {
		effects.track(runtimeID)
	}
}

func (s *subscriberState) onScanUpdate(
	added []procscan.DiscoveredProcess,
	removed []procscan.ProcessID,
	effects effects,
) {
	if log.ShouldLog(log.TraceLvl) {
		added := added
		removed := removed
		log.Tracef("process subscriber: onScanUpdate: added=%v, removed=%v", added, removed)
	}
	for _, proc := range added {
		runtimeID := proc.TracerMetadata.RuntimeID
		if runtimeID == "" {
			log.Debugf(
				"process subscriber: discovered process %d without runtime ID; skipping",
				proc.PID,
			)
			continue
		}
		pid := process.ID{PID: int32(proc.PID)}
		if _, ok := s.tracked[runtimeID]; !ok {
			s.tracked[runtimeID] = &runtimeEntry{
				Info: process.Info{
					ProcessID:   pid,
					Executable:  proc.Executable,
					Service:     proc.TracerMetadata.ServiceName,
					Environment: proc.TracerMetadata.ServiceEnv,
					Version:     proc.TracerMetadata.ServiceVersion,
				},
				runtimeID:    runtimeID,
				probesByPath: make(map[string]ir.ProbeDefinition),
			}
			log.Tracef(
				"process subscriber: discovered new runtime %s (pid=%d)",
				runtimeID, pid.PID,
			)
			if s.streamEstablished {
				effects.track(runtimeID)
			}
		}

		s.pidToRuntime[pid.PID] = runtimeID
	}

	var removals []process.ID
	for _, removedPID := range removed {
		pid := int32(removedPID)
		runtimeID, ok := s.pidToRuntime[pid]
		if !ok {
			continue
		}
		delete(s.pidToRuntime, pid)
		delete(s.tracked, runtimeID)
		removals = append(removals, process.ID{PID: pid})
		if s.streamEstablished {
			effects.untrack(runtimeID)
		}
		if log.ShouldLog(log.TraceLvl) {
			log.Tracef(
				"process subscriber: reporting removal for pid %d",
				pid,
			)
		}
	}

	if len(removals) > 0 {
		effects.emitUpdate(process.ProcessesUpdate{Removals: removals})
	}
}

func (s *subscriberState) onStreamConfig(
	resp *pbgo.ConfigSubscriptionResponse,
	effects effects,
) {
	if resp == nil || resp.Client == nil {
		return
	}

	tracer := resp.Client.GetClientTracer()
	if tracer == nil {
		return
	}
	runtimeID := tracer.GetRuntimeId()
	entry, ok := s.tracked[runtimeID]
	if !ok {
		return
	}
	if service := tracer.GetService(); service != "" {
		entry.Service = service
	}
	if env := tracer.GetEnv(); env != "" {
		entry.Environment = env
	}
	if version := tracer.GetAppVersion(); version != "" {
		entry.Version = version
	}
	if gitInfo := gitInfoFromTags(tracer.GetTags()); gitInfo != nil {
		entry.GitInfo = *gitInfo
	} else if gitInfo := gitInfoFromTags(tracer.GetProcessTags()); gitInfo != nil {
		entry.GitInfo = *gitInfo
	} else if gitInfo := gitInfoFromTags(tracer.GetContainerTags()); gitInfo != nil {
		entry.GitInfo = *gitInfo
	}
	if containerID := containerIDFromTracer(tracer); containerID != "" {
		entry.Container = process.ContainerInfo{
			ContainerID: containerID,
		}
	}

	if entry.probesByPath == nil {
		entry.probesByPath = make(map[string]ir.ProbeDefinition)
	}

	matchedSet := make(map[string]struct{}, len(resp.MatchedConfigs))
	symdbPathPresent := false
	for _, path := range resp.MatchedConfigs {
		matchedSet[path] = struct{}{}
		cfgPath, err := data.ParseConfigPath(path)
		if err != nil {
			log.Warnf(
				"process subscriber: runtime %s: failed to parse matched config path %q: %v",
				runtimeID, path, err,
			)
			continue
		}
		if cfgPath.Product == data.ProductLiveDebuggingSymbolDB {
			symdbPathPresent = true
		}
	}

	parsed := parseRemoteConfigFiles(runtimeID, resp.TargetFiles)

	var deletedAny bool
	for path := range entry.probesByPath {
		if _, ok := matchedSet[path]; !ok {
			delete(entry.probesByPath, path)
			deletedAny = true
		}
	}

	var addedAny bool
	for path, probe := range parsed.probes {
		if p, ok := entry.probesByPath[path]; !ok {
			addedAny = true
		} else if ir.CompareProbeIDs(p, probe) != 0 {
			addedAny = true
		}
		entry.probesByPath[path] = probe
	}

	previousSymdbEnabled := entry.symdbEnabled
	if parsed.haveSymdbFile {
		entry.symdbEnabled = parsed.symdbEnabled
	} else if !symdbPathPresent {
		entry.symdbEnabled = false
	}

	needProbesUpdate := addedAny || deletedAny
	symdbChanged := entry.symdbEnabled != previousSymdbEnabled

	if !needProbesUpdate && !symdbChanged {
		log.Tracef(
			"process subscriber: runtime %s configuration unchanged", runtimeID,
		)
		return
	}

	probes := slices.SortedFunc(maps.Values(entry.probesByPath), ir.CompareProbeIDs)
	cfg := process.Config{
		Info:              entry.Info,
		RuntimeID:         runtimeID,
		Probes:            probes,
		ShouldUploadSymDB: entry.symdbEnabled,
	}
	effects.emitUpdate(process.ProcessesUpdate{Updates: []process.Config{cfg}})
}
