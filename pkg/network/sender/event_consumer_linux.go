// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	ddos "github.com/DataDog/datadog-agent/pkg/util/os"
)

type directSenderConsumer struct {
	log       log.Component
	processes map[uint32]*process
	mtx       sync.Mutex

	proxyFilter          *dockerProxyFilter
	extractor            *serviceExtractor
	processNameExtractor *processNameExtractor
	pidAliveFunc         func(pid int) bool
	fetchProcesses       bool
}

func newDirectSenderConsumer(log log.Component, sysprobeconfig sysprobeconfig.Component) *directSenderConsumer {
	return &directSenderConsumer{
		log:                  log,
		processes:            make(map[uint32]*process),
		proxyFilter:          newDockerProxyFilter(log),
		extractor:            newServiceExtractor(sysprobeconfig),
		processNameExtractor: newProcessNameExtractor(),
		pidAliveFunc:         ddos.PidExists,
	}
}

// Copy implements eventmonitor.EventConsumerHandler
func (d *directSenderConsumer) Copy(ev *model.Event) any {
	p := &process{
		Pid:       ev.GetProcessPid(),
		PPid:      ev.GetProcessPpid(),
		EventType: ev.GetEventType(),
		Cmdline:   ev.GetExecCmdargv(),
	}
	return p
}

func (d *directSenderConsumer) handleNewProcess(p *process) {
	pidStr := strconv.Itoa(int(p.Pid))
	if p.Cwd == "" {
		cwd, err := os.Readlink(kernel.HostProc(pidStr, "cwd"))
		if err != nil && !os.IsNotExist(err) {
			if cwdLogLimiter.ShouldLog() {
				d.log.Warnf("error reading working directory for pid %d: %s", p.Pid, err)
			}
		}
		p.Cwd = cwd
	}

	if p.Comm == "" {
		comm, err := os.ReadFile(kernel.HostProc(pidStr, "comm"))
		if err != nil && !os.IsNotExist(err) {
			if cwdLogLimiter.ShouldLog() {
				d.log.Warnf("error reading comm for pid %d: %s", p.Pid, err)
			}
		}
		p.Comm = string(bytes.TrimSpace(comm))
	}

	if p.Exe == "" {
		exe, err := os.Readlink(kernel.HostProc(pidStr, "exe"))
		if err != nil && !os.IsNotExist(err) {
			if cwdLogLimiter.ShouldLog() {
				d.log.Warnf("error reading exe for pid %d: %s", p.Pid, err)
			}
		}
		p.Exe = exe
	}
}

func (d *directSenderConsumer) collectProcesses() error {
	if !d.fetchProcesses {
		return nil
	}

	rootProc := kernel.ProcFSRoot()
	pids, err := kernel.AllPidsProcs(rootProc)
	if err != nil {
		return err
	}

	for _, pid := range pids {
		pidPath := filepath.Join(rootProc, strconv.Itoa(pid))

		var ppid int64
		stat, err := os.ReadFile(filepath.Join(pidPath, "stat"))
		if err == nil {
			processNameEndIndex := bytes.LastIndexByte(stat, byte(')'))
			if processNameEndIndex > 0 && processNameEndIndex+1 < len(stat) {
				fieldNum := 0
				// start fields after process name
				for field := range bytes.FieldsSeq(stat[processNameEndIndex+1:]) {
					fieldNum++
					if fieldNum == 2 {
						ppid, _ = strconv.ParseInt(string(field), 10, 32)
						break
					}
				}
			}
		}

		var cmdline []string
		cmd, err := os.ReadFile(filepath.Join(pidPath, "cmdline"))
		if err == nil {
			cmd = bytes.TrimSpace(cmd)
			for cmdPiece := range bytes.SplitSeq(cmd, []byte{'\x00'}) {
				cmdline = append(cmdline, string(cmdPiece))
			}
		}

		p := &process{
			Pid:       uint32(pid),
			PPid:      uint32(ppid),
			Cmdline:   cmdline,
			EventType: model.ExecEventType,
		}
		d.HandleEvent(p)
	}

	return nil
}
