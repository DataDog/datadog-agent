// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/java"
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

type JavaTLSProgram struct {
	processMonitor *monitor.ProcessMonitor
	cleanupExec    func()
}

var _ subprogram = &JavaTLSProgram{}

func newJavaTLSProgram(c *config.Config) *JavaTLSProgram {
	if !c.EnableHTTPSMonitoring || !c.EnableJavaTLSSupport {
		return nil
	}

	if !c.EnableRuntimeCompiler {
		log.Errorf("goTLS support requires runtime-compilation to be enabled")
		return nil
	}

	mon := monitor.GetProcessMonitor()
	return &JavaTLSProgram{
		processMonitor: mon,
	}
}

func (p *JavaTLSProgram) ConfigureManager(m *nettelemetry.Manager) {
	if p == nil {
		return
	}

	//TODO setup the random id here
}

func (p *JavaTLSProgram) ConfigureOptions(options *manager.Options) {}

func (p *JavaTLSProgram) GetAllUndefinedProbes() (probeList []manager.ProbeIdentificationPair) {
	return
}

func newJavaProcess(pid uint32) {
	if err := java.InjectAgent(int(pid), "/opt/datadog-agent/embedded/share/system-probe/ebpf/java.agent.jar", ""); err != nil {
		log.Errorf("%v", err)
	}
}

func (p *JavaTLSProgram) Start() {
	if p == nil {
		return
	}

	var err error
	p.cleanupExec, err = p.processMonitor.Subscribe(&monitor.ProcessCallback{
		Event:    monitor.EXEC,
		Metadata: monitor.NAME,
		Regex:    regexp.MustCompile("^java$"),
		Callback: newJavaProcess,
	})
	if err != nil {
		log.Errorf("process monitor Subscribe() error: %s", err)
		return
	}
}

func (p *JavaTLSProgram) Stop() {
	if p == nil {
		return
	}
	p.cleanupExec()
}
