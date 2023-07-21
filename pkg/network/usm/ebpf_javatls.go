// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls/java"
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentUSMJar                 = "agent-usm.jar"
	javaTLSConnectionsMap       = "java_tls_connections"
	javaDomainsToConnectionsMap = "java_conn_tuple_by_peer"
	eRPCHandlersMap             = "java_tls_erpc_handlers"

	doVfsIoctlKprobeName             = "kprobe__do_vfs_ioctl"
	handleSyncPayloadKprobeName      = "kprobe_handle_sync_payload"
	handleCloseConnectionKprobeName  = "kprobe_handle_close_connection"
	handleConnectionByPeerKprobeName = "kprobe_handle_connection_by_peer"
	handleAsyncPayloadKprobeName     = "kprobe_handle_async_payload"
)

const (
	// syncPayload is the key to the program that handles the SYNCHRONOUS_PAYLOAD eRPC operation
	syncPayload uint32 = iota
	// closeConnection is the key to the program that handles the CLOSE_CONNECTION eRPC operation
	closeConnection
	// connectionByPeer is the key to the program that handles the CONNECTION_BY_PEER eRPC operation
	connectionByPeer
	// asyncPayload is the key to the program that handles the ASYNC_PAYLOAD eRPC operation
	asyncPayload
)

var (
	javaProcessName = []byte("java")
)

type javaTLSProgram struct {
	cfg            *config.Config
	processMonitor *monitor.ProcessMonitor
	cleanupExec    func()

	// tracerJarPath path to the USM agent TLS tracer.
	tracerJarPath string

	// tracerArguments default arguments passed to the injected agent-usm.jar
	tracerArguments string

	// injectionAllowRegex is matched against /proc/pid/cmdline, to determine if we should attach to the process.
	injectionAllowRegex *regexp.Regexp
	// injectionAllowRegex is matched against /proc/pid/cmdline, to determine if we should deny attachment to the process.
	injectionBlockRegex *regexp.Regexp

	procRoot string
}

// Static evaluation to make sure we are not breaking the interface.
var _ subprogram = &javaTLSProgram{}

func getJavaTlsTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: eRPCHandlersMap,
			Key:           syncPayload,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: handleSyncPayloadKprobeName,
			},
		},
		{
			ProgArrayName: eRPCHandlersMap,
			Key:           closeConnection,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: handleCloseConnectionKprobeName,
			},
		},
		{
			ProgArrayName: eRPCHandlersMap,
			Key:           connectionByPeer,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: handleConnectionByPeerKprobeName,
			},
		},
		{
			ProgArrayName: eRPCHandlersMap,
			Key:           asyncPayload,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: handleAsyncPayloadKprobeName,
			},
		},
	}
}

func newJavaTLSProgram(c *config.Config) *javaTLSProgram {
	var res *javaTLSProgram
	defer func() {
		// If we didn't set res, then java tls initialization failed.
		if res == nil {
			log.Info("java tls is not enabled")
		}
	}()

	if !c.EnableJavaTLSSupport || !c.EnableHTTPSMonitoring || !http.HTTPSSupported(c) {
		return nil
	}

	javaUSMAgentJarPath := filepath.Join(c.JavaDir, agentUSMJar)
	// We tried switching os.Open to os.Stat, but it seems it does not guarantee we'll be able to copy the file.
	if f, err := os.Open(javaUSMAgentJarPath); err != nil {
		log.Errorf("java TLS can't access java tracer payload %s : %s", javaUSMAgentJarPath, err)
		return nil
	} else {
		// If we managed to open the file, then we close it, as we just needed to check if the file exists.
		_ = f.Close()
	}

	log.Info("java tls is enabled")

	res = &javaTLSProgram{
		cfg:                 c,
		processMonitor:      monitor.GetProcessMonitor(),
		tracerArguments:     buildTracerArguments(c),
		tracerJarPath:       javaUSMAgentJarPath,
		injectionAllowRegex: buildRegex(c.JavaAgentAllowRegex, "allow"),
		injectionBlockRegex: buildRegex(c.JavaAgentBlockRegex, "block"),
		procRoot:            util.GetProcRoot(),
	}

	return res
}

func (p *javaTLSProgram) Name() string {
	return "java-tls"
}

func (p *javaTLSProgram) IsBuildModeSupported(buildMode) bool {
	return true
}

func (p *javaTLSProgram) ConfigureManager(m *nettelemetry.Manager) {
	m.Maps = append(m.Maps, []*manager.Map{
		{Name: javaTLSConnectionsMap},
	}...)

	m.Probes = append(m.Probes,
		&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{
			EBPFFuncName: doVfsIoctlKprobeName,
			UID:          probeUID,
		},
			KProbeMaxActive: maxActive,
		},
	)
}

func (p *javaTLSProgram) ConfigureOptions(options *manager.Options) {
	options.MapSpecEditors[javaTLSConnectionsMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: p.cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}
	options.MapSpecEditors[javaDomainsToConnectionsMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: p.cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}

	options.ActivatedProbes = append(options.ActivatedProbes,
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: doVfsIoctlKprobeName,
				UID:          probeUID,
			},
		})
}

func (*javaTLSProgram) GetAllUndefinedProbes() []manager.ProbeIdentificationPair {
	return []manager.ProbeIdentificationPair{
		{EBPFFuncName: doVfsIoctlKprobeName},
		{EBPFFuncName: handleSyncPayloadKprobeName},
		{EBPFFuncName: handleCloseConnectionKprobeName},
		{EBPFFuncName: handleConnectionByPeerKprobeName},
		{EBPFFuncName: handleAsyncPayloadKprobeName},
	}
}

// isJavaProcess checks if the given PID comm's name is java.
// The method is much faster and efficient that using process.NewProcess(pid).Name().
func (p *javaTLSProgram) isJavaProcess(pid uint32) bool {
	filePath := filepath.Join(p.procRoot, strconv.Itoa(int(pid)), "comm")
	content, err := os.ReadFile(filePath)
	if err != nil {
		// Waiting a bit, as we might get the event of process creation before the directory was created.
		for i := 0; i < 3; i++ {
			time.Sleep(10 * time.Millisecond)
			// reading again.
			content, err = os.ReadFile(filePath)
			if err == nil {
				break
			}
		}
	}

	if err != nil {
		// short living process can hit here, or slow start of another process.
		return false
	}
	return bytes.Equal(bytes.TrimSpace(content), javaProcessName)
}

// isAttachmentAllowed will return true if the pid can be attached
// The filter is based on the process command line matching injectionAllowRegex and injectionBlockRegex regex
// injectionAllowRegex has a higher priority
//
// # In case of only one regex (allow or block) is set, the regex will be evaluated as exclusive filter
// /                 match  | not match
// allowRegex only    true  | false
// blockRegex only    false | true
func (p *javaTLSProgram) isAttachmentAllowed(pid uint32) bool {
	allowIsSet := p.injectionAllowRegex != nil
	blockIsSet := p.injectionBlockRegex != nil
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
	if allowIsSet && p.injectionAllowRegex.MatchString(fullCmdline) {
		return true
	}
	if blockIsSet && p.injectionBlockRegex.MatchString(fullCmdline) {
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

func (p *javaTLSProgram) newJavaProcess(pid uint32) {
	if !p.isJavaProcess(pid) {
		return
	}
	if !p.isAttachmentAllowed(pid) {
		log.Debugf("java pid %d attachment rejected", pid)
		return
	}

	if err := java.InjectAgent(int(pid), p.tracerJarPath, p.tracerArguments); err != nil {
		log.Error(err)
	}
}

func (p *javaTLSProgram) Start() {
	p.cleanupExec = p.processMonitor.SubscribeExec(p.newJavaProcess)
}

func (p *javaTLSProgram) Stop() {
	if p.cleanupExec != nil {
		p.cleanupExec()
	}

	if p.processMonitor != nil {
		p.processMonitor.Stop()
	}
}

// buildRegex is similar to regexp.MustCompile, but without panic.
func buildRegex(re, reType string) *regexp.Regexp {
	if re == "" {
		return nil
	}
	res, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("%s regex can't be compiled %s", reType, err)
		return nil
	}

	return res
}

// buildTracerArguments returns the command line arguments we'll pass to the injected tracer.
func buildTracerArguments(c *config.Config) string {
	// Randomizing the seed to ensure we get a truly random number.
	rand.Seed(int64(os.Getpid()) + time.Now().UnixMicro())

	allArgs := []string{
		c.JavaAgentArgs,
		// authID is used here as an identifier, simple proof of authenticity
		// between the injected java process and the ebpf ioctl that receive the payload.
		fmt.Sprintf("dd.usm.authID=%d", rand.Int63()),
	}
	if c.JavaAgentDebug {
		allArgs = append(allArgs, "dd.trace.debug=true")
	}
	return strings.Join(allArgs, ",")
}
