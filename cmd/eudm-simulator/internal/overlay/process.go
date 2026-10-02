// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package overlay

import (
	"errors"
	"fmt"
	"math"
	"slices"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

type processChange struct {
	before            *model.Process
	cpu               model.CPUStat
	rss               uint64
	cpus, totalMemory float64
}

func processChanges(ctx Context, def schema.ProcessDef) ([]processChange, error) {
	var matches []*model.Process
	var cpus, totalMemory, baseCPU, baseMemory float64
	for _, chunk := range ctx.BaselineProcesses {
		if chunk == nil || chunk.Info == nil {
			return nil, errors.New("process overlay requires complete captured system information")
		}
		var chunkCPUs float64
		for _, cpu := range chunk.Info.Cpus {
			if cpu != nil {
				chunkCPUs += float64(cpu.Cores)
			}
		}
		if chunkCPUs <= 0 || chunk.Info.TotalMemory <= 0 {
			return nil, errors.New("process overlay requires captured CPU topology and memory capacity")
		}
		if cpus != 0 && (cpus != chunkCPUs || totalMemory != float64(chunk.Info.TotalMemory)) {
			return nil, errors.New("process cycle has inconsistent system information")
		}
		cpus, totalMemory = chunkCPUs, float64(chunk.Info.TotalMemory)
		for _, process := range chunk.Processes {
			if process == nil || process.Command == nil || process.Command.Comm != def.Name {
				continue
			}
			if process.Cpu == nil || process.Memory == nil {
				return nil, fmt.Errorf("process %q lacks captured resource evidence", def.Name)
			}
			matches = append(matches, process)
			baseCPU += float64(process.Cpu.TotalPct)
			baseMemory += float64(process.Memory.Rss)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("process %q is absent from captured cycle", def.Name)
	}
	slices.SortFunc(matches, func(a, b *model.Process) int {
		if a.Pid < b.Pid {
			return -1
		}
		if a.Pid > b.Pid {
			return 1
		}
		return 0
	})
	for i := 1; i < len(matches); i++ {
		if matches[i-1].Pid == matches[i].Pid {
			return nil, errors.New("captured process cycle repeats a PID")
		}
	}
	key := ctx
	key.Stream, key.SampleOrdinal = schema.Processes, ctx.ProcessSampleOrdinal
	cpu, err := PatternValue(key, def.CPU, "process/"+def.Name+"/cpu")
	if err != nil {
		return nil, err
	}
	memory, err := PatternValue(key, def.Memory, "process/"+def.Name+"/memory")
	if err != nil {
		return nil, err
	}
	if cpu < 0 || cpu > 100 || memory < 0 || memory*megabyte > totalMemory {
		return nil, errors.New("process overlay exceeds captured resource capacity")
	}
	targetCPU, targetMemory := cpu*cpus, uint64(math.Round(memory*megabyte))
	changes := make([]processChange, 0, len(matches))
	var allocated uint64
	for i, process := range matches {
		cpuShare, memoryShare := 1/float64(len(matches)), 1/float64(len(matches))
		if baseCPU > 0 {
			cpuShare = float64(process.Cpu.TotalPct) / baseCPU
		}
		if baseMemory > 0 {
			memoryShare = float64(process.Memory.Rss) / baseMemory
		}
		newMemory := uint64(math.Floor(float64(targetMemory) * memoryShare))
		if i == len(matches)-1 {
			newMemory = targetMemory - allocated
		}
		allocated += newMemory
		userShare := 1.0
		if sum := float64(process.Cpu.UserPct) + float64(process.Cpu.SystemPct); sum > 0 {
			userShare = float64(process.Cpu.UserPct) / sum
		}
		stat := *process.Cpu
		stat.TotalPct = float32(targetCPU * cpuShare)
		stat.UserPct = stat.TotalPct * float32(userShare)
		stat.SystemPct = stat.TotalPct - stat.UserPct
		changes = append(changes, processChange{before: process, cpu: stat, rss: newMemory, cpus: cpus, totalMemory: totalMemory})
	}
	return changes, nil
}

func applyProcesses(ctx Context, payload *model.CollectorProc, defs []schema.ProcessDef) error {
	for _, def := range defs {
		changes, err := processChanges(ctx, def)
		if err != nil {
			return err
		}
		byPID := map[int32]processChange{}
		for _, change := range changes {
			byPID[change.before.Pid] = change
		}
		for _, process := range payload.Processes {
			if process == nil || process.Command == nil || process.Command.Comm != def.Name {
				continue
			}
			change, exists := byPID[process.Pid]
			if !exists || process.Memory == nil {
				return errors.New("process clone differs from its captured cycle")
			}
			cpu := change.cpu
			process.Cpu = &cpu
			if process.Memory.Vms > 0 {
				process.Memory.Vms = uint64(max(float64(change.rss), float64(process.Memory.Vms)+float64(change.rss)-float64(process.Memory.Rss)))
			}
			process.Memory.Rss = change.rss
			if def.User != "" && process.User != nil {
				process.User.Name = def.User
			}
			if def.Exe != "" {
				process.Command.Exe = def.Exe
			}
			if def.Args != nil {
				process.Command.Args = slices.Clone(def.Args)
			}
			// SentinelOne includes its release in the installation directory.
			// Chrome's executable path is stable across releases.
			if def.Name == "SentinelAgent.exe" {
				if item, ok := softwareItem(ctx, "SentinelOne"); ok {
					process.Command.Exe = sentinelDirectory(item.Version) + `\SentinelAgent.exe`
				}
			}
			if process.Command.Exe != "" && len(process.Command.Args) > 0 {
				process.Command.Args[0] = process.Command.Exe
			}
		}
	}
	return nil
}

func softwareItem(ctx Context, name string) (schema.SoftwareItem, bool) {
	for _, item := range ctx.Scenario.Phases[ctx.PhaseIndex].Software[ctx.Group.Group] {
		if item.Name == name {
			return item, true
		}
	}
	for _, item := range ctx.Scenario.Software[ctx.Group.Group] {
		if item.Name == name {
			return item, true
		}
	}
	return schema.SoftwareItem{}, false
}

func sentinelDirectory(version string) string {
	return `C:\Program Files\SentinelOne\Sentinel Agent ` + version
}

func applySoftware(ctx Context, sample *telemetry.Sample) error {
	items := map[string]schema.SoftwareItem{}
	for _, item := range ctx.Scenario.Software[ctx.Group.Group] {
		items[item.Name] = item
	}
	for _, item := range ctx.Scenario.Phases[ctx.PhaseIndex].Software[ctx.Group.Group] {
		items[item.Name] = item
	}
	if len(items) == 0 {
		return nil
	}
	if sample.Software == nil {
		return errors.New("software overlay requires a captured snapshot")
	}
	for name, item := range items {
		found := false
		for i := range sample.Software.Metadata.Software {
			entry := &sample.Software.Metadata.Software[i]
			if entry.DisplayName != name {
				continue
			}
			found = true
			entry.Version = item.Version
			if item.Publisher != "" {
				entry.Publisher = item.Publisher
			}
			if item.SoftwareType != "" {
				entry.Source = item.SoftwareType
			}
			if item.DeploymentStatus != "" {
				entry.Status = item.DeploymentStatus
			}
			if item.DeploymentTime != "" {
				entry.InstallDate = item.DeploymentTime
			}
			if item.ProductCode != "" {
				entry.ProductCode = item.ProductCode
			}
			if item.User != "" {
				entry.UserSID = item.User
			}
			if item.Is64Bit {
				entry.Is64Bit = true
			}
			if name == "SentinelOne" {
				for j := range entry.InstallPaths {
					entry.InstallPaths[j] = sentinelDirectory(item.Version)
				}
			}
		}
		if !found {
			return fmt.Errorf("software %q is absent from captured snapshot", name)
		}
	}
	return nil
}
