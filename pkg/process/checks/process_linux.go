// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	workloadmetacomp "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// useWLMCollection checks the configuration to use the workloadmeta process collector or not in linux
// TODO: process_config.process_collection.use_wlm is a temporary configuration for refactoring purposes
func (p *ProcessCheck) useWLMCollection() bool {
	return p.config.GetBool("process_config.process_collection.use_wlm")
}

// processesByPID returns the processes by pid from different sources depending on the configuration (process probe or workloadmeta)
// workload meta process collection is only available on linux and TODO: will eventually be the only source for linux process collection
func (p *ProcessCheck) processesByPID(collectStats bool) (map[int32]*procutil.Process, error) {
	if p.useWLMProcessCollection {
		wlmProcList := p.wmeta.ListProcesses()
		pids := make([]int32, len(wlmProcList))
		for i, wlmProc := range wlmProcList {
			pids[i] = wlmProc.Pid
		}

		statsForProcess := make(map[int32]*procutil.Stats)
		if collectStats {
			var err error
			statsForProcess, err = p.probe.StatsForPIDs(pids, p.clock.Now())
			if err != nil {
				return nil, err
			}
		}

		// map to common process type used by other versions of the check
		procs := make(map[int32]*procutil.Process, len(wlmProcList))
		for _, wlmProc := range wlmProcList {
			procs[wlmProc.Pid] = mapWLMProcToProc(wlmProc, statsForProcess[wlmProc.Pid])
		}
		return procs, nil
	}
	procs, err := p.probe.ProcessesByPID(p.clock.Now(), collectStats)
	if err != nil {
		return nil, err
	}
	return procs, nil
}

func mapWLMProcToProc(wlmProc *workloadmetacomp.Process, stats *procutil.Stats) *procutil.Process {
	return &procutil.Process{
		Pid:     wlmProc.Pid,
		Ppid:    wlmProc.Ppid,
		NsPid:   wlmProc.NsPid,
		Name:    wlmProc.Name,
		Cwd:     wlmProc.Cwd,
		Exe:     wlmProc.Exe,
		Comm:    wlmProc.Comm,
		Cmdline: wlmProc.Cmdline,
		Uids:    wlmProc.Uids,
		Gids:    wlmProc.Gids,
		Stats:   stats,
	}
}
