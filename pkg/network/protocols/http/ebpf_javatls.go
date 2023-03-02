// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/java"
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	agentUSMJar           = "agent-usm.jar"
	javaTLSConnectionsMap = "java_tls_connections"
)

var (
	// path to our java USM agent TLS tracer
	javaUSMAgentJarPath = ""

	// enable debug output in the injected agent-usm.jar
	javaUSMAgentDebug = false

	// default arguments passed to the injected agent-usm.jar
	javaUSMAgentArgs = ""

	// authID is used here as an identifier, simple proof of authenticity
	// between the injected java process and the ebpf ioctl that receive the payload
	authID = int64(0)

	// The regex is matching against /proc/pid/cmdline
	// if matching the agent-usm.jar would or not injected
	javaAgentAllowRegex *regexp.Regexp
	javaAgentBlockRegex *regexp.Regexp
)

type JavaTLSProgram struct {
	cfg            *config.Config
	manager        *nettelemetry.Manager
	processMonitor *monitor.ProcessMonitor
	cleanupExec    func()
}

// Static evaluation to make sure we are not breaking the interface.
var _ subprogram = &JavaTLSProgram{}

func newJavaTLSProgram(c *config.Config) *JavaTLSProgram {
	var err error

	if !c.EnableJavaTLSSupport || !c.EnableHTTPSMonitoring || !HTTPSSupported(c) {
		log.Info("java tls is not enabled")
		return nil
	}

	log.Info("java tls is enabled")
	javaUSMAgentJarPath = filepath.Join(c.JavaDir, agentUSMJar)
	javaUSMAgentDebug = c.JavaAgentDebug
	javaUSMAgentArgs = c.JavaAgentArgs

	javaAgentAllowRegex = nil
	javaAgentBlockRegex = nil
	if c.JavaAgentAllowRegex != "" {
		javaAgentAllowRegex, err = regexp.Compile(c.JavaAgentAllowRegex)
		if err != nil {
			javaAgentAllowRegex = nil
			log.Errorf("JavaAgentAllowRegex regex can't be compiled %s", err)
		}
	}
	if c.JavaAgentBlockRegex != "" {
		javaAgentBlockRegex, err = regexp.Compile(c.JavaAgentBlockRegex)
		if err != nil {
			javaAgentBlockRegex = nil
			log.Errorf("JavaAgentBlockRegex regex can't be compiled %s", err)
		}
	}

	jar, err := os.Open(javaUSMAgentJarPath)
	if err != nil {
		log.Errorf("java TLS can't access to agent-usm.jar file %s : %s", javaUSMAgentJarPath, err)
		return nil
	}
	jar.Close()

	mon := monitor.GetProcessMonitor()
	return &JavaTLSProgram{
		cfg:            c,
		processMonitor: mon,
	}
}

func (p *JavaTLSProgram) ConfigureManager(m *nettelemetry.Manager) {
	p.manager = m
	p.manager.Maps = append(p.manager.Maps, []*manager.Map{
		{Name: javaTLSConnectionsMap},
	}...)

	p.manager.Probes = append(m.Probes,
		&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{
			EBPFFuncName: "kprobe__do_vfs_ioctl",
			UID:          probeUID,
		},
			KProbeMaxActive: maxActive,
		},
	)
	rand.Seed(int64(os.Getpid()) + time.Now().UnixMicro())
	authID = rand.Int63()
}

func (p *JavaTLSProgram) ConfigureOptions(options *manager.Options) {
	options.MapSpecEditors[javaTLSConnectionsMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: uint32(p.cfg.MaxTrackedConnections),
		EditorFlag: manager.EditMaxEntries,
	}
	options.ActivatedProbes = append(options.ActivatedProbes,
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe__do_vfs_ioctl",
				UID:          probeUID,
			},
		})
}

func (p *JavaTLSProgram) GetAllUndefinedProbes() []manager.ProbeIdentificationPair {
	return []manager.ProbeIdentificationPair{{EBPFFuncName: "kprobe__do_vfs_ioctl"}}
}

// isAttachmentAllowed will return true if the pid can be attached
// The filter is based on the process command line matching javaAgentAllowRegex and javaAgentBlockRegex regex
// javaAgentAllowRegex has a higher priority
//
// # In case of only one regex (allow or block) is set, the regex will be evaluated as exclusive filter
// /                 match  | not match
// allowRegex only    true  | false
// blockRegex only    false | true
func isAttachmentAllowed(pid uint32) bool {
	allowIsSet := javaAgentAllowRegex != nil
	blockIsSet := javaAgentBlockRegex != nil
	// filter is disabled (default configuration)
	if !allowIsSet && !blockIsSet {
		return true
	}

	procCmdline := fmt.Sprintf("%s/%d/cmdline", util.HostProc(), pid)
	cmd, err := os.ReadFile(procCmdline)
	if err != nil {
		log.Debugf("injection filter can't open commandline %q : %s", procCmdline, err)
		return false
	}
	fullCmdline := strings.ReplaceAll(string(cmd), "\000", " ") // /proc/pid/cmdline format : arguments are separated by '\0'

	// Allow to have a higher priority
	if allowIsSet && javaAgentAllowRegex.MatchString(fullCmdline) {
		return true
	}
	if blockIsSet && javaAgentBlockRegex.MatchString(fullCmdline) {
		return false
	}

	// if only one regex is set, allow regex if not match should not attach
	if allowIsSet != blockIsSet { // allow xor block
		if allowIsSet {
			return false
		}
	}
	return true
}

func newJavaProcess(pid uint32) {
	if !isAttachmentAllowed(pid) {
		log.Debugf("java pid %d attachment rejected", pid)
		return
	}

	allArgs := []string{
		javaUSMAgentArgs,
		"dd.usm.authID=" + strconv.FormatInt(authID, 10),
	}
	if javaUSMAgentDebug {
		allArgs = append(allArgs, "dd.trace.debug=true")
	}
	args := strings.Join(allArgs, " ")
	if err := java.InjectAgent(int(pid), javaUSMAgentJarPath, args); err != nil {
		log.Error(err)
	}
}

func (p *JavaTLSProgram) Start() {
	var err error
	p.cleanupExec, err = p.processMonitor.Subscribe(&monitor.ProcessCallback{
		Event:    monitor.EXEC,
		Metadata: monitor.NAME,
		Regex:    regexp.MustCompile("^java$"),
		Callback: newJavaProcess,
	})
	if err != nil {
		log.Errorf("process monitor Subscribe() error: %s", err)
	}
}

func (p *JavaTLSProgram) Stop() {
	if p.cleanupExec != nil {
		p.cleanupExec()
	}
}
