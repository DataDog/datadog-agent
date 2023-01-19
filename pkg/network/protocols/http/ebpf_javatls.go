// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/java"
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	AgentUSMJar = "agent-usm.jar"
)

var (
	// path to our java USM agent TLS tracer
	javaUSMAgentJarPath = ""
	// randomID is used here as an identifier, simple proof of authenticity
	// between the injected java process and the ebpf ioctl that receive the payload
	randomID = int64(0)
)

type JavaTLSProgram struct {
	processMonitor *monitor.ProcessMonitor
	cleanupExec    func()
}

// Static evaluation to make sure we are not breaking the interface.
var _ subprogram = &JavaTLSProgram{}

func newJavaTLSProgram(c *config.Config) *JavaTLSProgram {
	if !c.EnableHTTPSMonitoring || !c.EnableJavaTLSSupport {
		return nil
	}

	if !c.EnableRuntimeCompiler {
		log.Errorf("java TLS support requires runtime-compilation to be enabled")
		return nil
	}
	javaUSMAgentJarPath = filepath.Join(c.JavaDir, AgentUSMJar)

	mon := monitor.GetProcessMonitor()
	return &JavaTLSProgram{
		processMonitor: mon,
	}
}

func (p *JavaTLSProgram) ConfigureManager(m *nettelemetry.Manager) {
	if p == nil {
		return
	}

	rand.Seed(int64(os.Getpid()) + time.Now().UnixMicro())
	randomID = rand.Int63()
}

func (p *JavaTLSProgram) ConfigureOptions(options *manager.Options) {}

func (p *JavaTLSProgram) GetAllUndefinedProbes() (probeList []manager.ProbeIdentificationPair) {
	return
}

func newJavaProcess(pid uint32) {
	if err := java.InjectAgent(int(pid), javaUSMAgentJarPath, strconv.FormatInt(randomID, 10)); err != nil {
		log.Error(err)
	}
}

func (p *JavaTLSProgram) Start() {
	if p == nil {
		log.Debugf("java TLS ebpf subprogram is not enabled")
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
		log.Debugf("java TLS ebpf subprogram is not enabled")
		return
	}
	if p.cleanupExec != nil {
		p.cleanupExec()
	}
}
