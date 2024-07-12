// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"bufio"
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cihub/seelog"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ExcludeMode uint8

const (
	ExcludeSelf ExcludeMode = 1 << iota
	ExcludeInternal
	ExcludeBuildkit
	ExcludeContainerdTmp
)

const procFSUpdateTimeout = 10 * time.Millisecond

var (
	// ErrSelfExcluded is returned when the PID is the same as the agent's PID.
	ErrSelfExcluded = errors.New("self-excluded")
	// ErrInternalDDogProcessRejected is returned when the PID is an internal datadog process.
	ErrInternalDDogProcessRejected = errors.New("internal datadog process rejected")
	// ErrNoMatchingRule is returned when no rule matches the shared library path.
	ErrNoMatchingRule = errors.New("no matching rule")
	// regex that defines internal DataDog processes
	internalProcessRegex = regexp.MustCompile("datadog-agent/.*/((process|security|trace)-agent|system-probe|agent)")
)

type AttachRules struct {
	UprobeNameRegex  *regexp.Regexp
	BinaryNameRegex  *regexp.Regexp
	ManualReturnHook bool
}

type AttacherConfig struct {
	// ProbesSelectors defines which probes should be attached and how should we validate
	// the attachment (e.g., whether we need all probes active or just one of them, or in a best-effort basis)
	ProbeSelectors []manager.ProbesSelector

	// Rules defines a series of rules that tell the attacher how to attach the probes
	Rules []*AttachRules

	// ScanTerminatedProcessesInterval defines the interval at which we scan for terminated processes. Set
	// to zero to disable
	ScanTerminatedProcessesInterval time.Duration

	// ProcRoot is the root directory of the proc filesystem
	ProcRoot string

	// ExcludeTargets defines the targets that should be excluded from the attacher
	ExcludeTargets ExcludeMode

	// EbpfConfig is the configuration for the eBPF program
	EbpfConfig *ebpf.Config
}

// SetDefaults configures the AttacherConfig with default values for those fields for which the compiler
// defaults are not enough
func (ac *AttacherConfig) SetDefaults() {
	if ac.ScanTerminatedProcessesInterval == 0 {
		ac.ScanTerminatedProcessesInterval = 30 * time.Minute
	}

	if ac.ProcRoot == "" {
		ac.ProcRoot = "/proc"
	}

	if ac.EbpfConfig == nil {
		ac.EbpfConfig = ebpf.NewConfig()
	}
}

// ProbeManager is an interface that defines the methods that a Manager implements,
// so that we can replace it in tests for a mock object
type ProbeManager interface {
	AddHook(string, *manager.Probe) error
	DetachHook(manager.ProbeIdentificationPair) error
	GetProbe(manager.ProbeIdentificationPair) (*manager.Probe, bool)
}

type UprobeAttacher struct {
	name         string
	done         chan struct{}
	wg           sync.WaitGroup
	config       *AttacherConfig
	fileRegistry *utils.FileRegistry
	manager      ProbeManager

	// ruleCache is a cache of the rules that match a given uprobe, to avoid computing that on every
	// attach operation
	ruleCache map[string][]*AttachRules

	// pathToAttachedProbes maps a filesystem path to the probes attached to it. Used to detach them
	// once the path is no longer used.
	pathToAttachedProbes map[string][]manager.ProbeIdentificationPair
	onAttachCallback     func(*manager.Probe)
	soWatcher            *sharedlibraries.EbpfProgram
	thisPID              int
}

func NewUprobeAttacher(name string, config *AttacherConfig, mgr ProbeManager, onAttachCallback func(*manager.Probe)) *UprobeAttacher {
	config.SetDefaults()

	return &UprobeAttacher{
		name:                 name,
		config:               config,
		fileRegistry:         utils.NewFileRegistry(name),
		manager:              mgr,
		onAttachCallback:     onAttachCallback,
		ruleCache:            make(map[string][]*AttachRules),
		pathToAttachedProbes: make(map[string][]manager.ProbeIdentificationPair),
		soWatcher:            sharedlibraries.NewEBPFProgram(config.EbpfConfig),
	}
}

func (ua *UprobeAttacher) Start() error {
	procMonitor := monitor.GetProcessMonitor()
	cleanupExec := procMonitor.SubscribeExec(ua.handleProcessStart)
	cleanupExit := procMonitor.SubscribeExit(ua.handleProcessExit)

	err := ua.soWatcher.Init()
	if err != nil {
		return fmt.Errorf("error initializing shared library program: %w", err)
	}

	err = ua.initialScan()
	if err != nil {
		return fmt.Errorf("error during initial scan: %w", err)
	}

	ua.wg.Add(1)
	go func() {
		processSync := time.NewTicker(ua.config.ScanTerminatedProcessesInterval)

		defer func() {
			processSync.Stop()
			cleanupExec()
			cleanupExit()
			procMonitor.Stop()
			ua.fileRegistry.Clear()
			ua.wg.Done()
		}()

		sharedLibDataChan := ua.soWatcher.GetPerfHandler().DataChannel()
		sharedLibLostChan := ua.soWatcher.GetPerfHandler().DataChannel()

		for {
			select {
			case <-ua.done:
				return
			case <-processSync.C:
				processSet := ua.fileRegistry.GetRegisteredProcesses()
				deletedPids := monitor.FindDeletedProcesses(processSet)
				for deletedPid := range deletedPids {
					ua.handleProcessExit(deletedPid)
				}
			case event, ok := <-sharedLibDataChan:
				if !ok {
					return
				}
				ua.handleLibraryOpen(event)
			case <-sharedLibLostChan:
				// Nothing to do in this case
				break
			}
		}
	}()

	return nil
}

func (ua *UprobeAttacher) initialScan() error {
	thisPID, err := kernel.RootNSPID()
	if err != nil {
		return fmt.Errorf("error getting PID of our own process: %w", err)
	}

	err = kernel.WithAllProcs(ua.config.ProcRoot, func(pid int) error {
		if pid == thisPID { // don't scan ourself
			return nil
		}

		return ua.AttachPID(uint32(pid), true)
	})

	return err
}

// handleProcessStart is called when a new process is started, wraps AttachPID but ignoring the error
// for API compatibility with processMonitor
func (ua *UprobeAttacher) handleProcessStart(pid uint32) {
	_ = ua.AttachPID(pid, false) // Do not try to attach to libraries on process start, it hasn't loaded them yet
}

// handleProcessExit is called when a process finishes, wraps DetachPID but ignoring the error
// for API compatibility with processMonitor
func (ua *UprobeAttacher) handleProcessExit(pid uint32) {
	_ = ua.DetachPID(pid)
}

func (ua *UprobeAttacher) handleLibraryOpen(event *ebpf.DataEvent) error {
	defer event.Done()

	libpath := sharedlibraries.ToLibPath(event.Data)
	path := sharedlibraries.ToBytes(&libpath)

	return ua.AttachLibrary(string(path), libpath.Pid)

}

func (ua *UprobeAttacher) AttachLibrary(path string, pid uint32) error {
	if int(pid) == os.Getpid() {
		return ErrSelfExcluded
	}

	matchingRules := ua.getRulesForLibrary(path)
	if len(matchingRules) == 0 {
		return ErrNoMatchingRule
	}

	registerCB := func(path utils.FilePath) error {
		return ua.attachToBinary(path, matchingRules)
	}
	unregisterCB := func(path utils.FilePath) error {
		return ua.detachFromBinary(path)
	}

	return ua.fileRegistry.Register(path, pid, registerCB, unregisterCB)
}

func (ua *UprobeAttacher) getRulesForLibrary(path string) []*AttachRules {
	var matchedRules []*AttachRules

	for _, rule := range ua.config.Rules {
		if rule.BinaryNameRegex.MatchString(path) {
			matchedRules = append(matchedRules, rule)
		}
	}
	return matchedRules
}

// getExecutablePath resolves the executable of the given PID looking in procfs. Automatically
// handles delays in procfs updates. Will return an error if the path cannot be resolved
func (ua *UprobeAttacher) getExecutablePath(pid uint32) (string, error) {
	pidAsStr := strconv.FormatUint(uint64(pid), 10)
	exePath := filepath.Join(ua.config.ProcRoot, pidAsStr, "exe")

	var binPath string
	err := errors.New("iteration start")
	end := time.Now().Add(procFSUpdateTimeout)

	for err != nil && end.After(time.Now()) {
		binPath, err = os.Readlink(exePath)
		if err != nil {
			time.Sleep(time.Millisecond)
		}
	}

	if err != nil {
		return "", err
	}

	return binPath, nil
}

func (ua *UprobeAttacher) AttachPID(pid uint32, attachToLibs bool) error {
	if (ua.config.ExcludeTargets&ExcludeSelf) != 0 && int(pid) == os.Getpid() {
		return ErrSelfExcluded
	}

	binPath, err := ua.getExecutablePath(pid)
	if err != nil {
		return err
	}

	if (ua.config.ExcludeTargets&ExcludeInternal) != 0 && internalProcessRegex.MatchString(binPath) {
		if log.ShouldLog(seelog.DebugLvl) {
			log.Debugf("ignoring pid %d, as it is an internal datadog component (%q)", pid, binPath)
		}
		return ErrInternalDDogProcessRejected
	}

	registerCB := func(path utils.FilePath) error {
		return ua.attachToBinary(path, nil)
	}
	unregisterCB := func(path utils.FilePath) error {
		return ua.detachFromBinary(path)
	}

	err = ua.fileRegistry.Register(binPath, pid, registerCB, unregisterCB)
	if err != nil {
		return err
	}

	if attachToLibs {
		err = ua.attachToLibrariesOfPID(pid)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ua *UprobeAttacher) DetachPID(pid uint32) error {
	return ua.fileRegistry.Unregister(pid)
}

const (
	// Defined in https://man7.org/linux/man-pages/man5/proc.5.html.
	taskCommLen = 16
)

var (
	taskCommLenBufferPool = sync.Pool{
		New: func() any {
			buf := make([]byte, taskCommLen)
			return &buf
		},
	}
	buildKitProcessName = []byte("buildkitd")
)

func isBuildKit(procRoot string, pid uint32) bool {
	filePath := filepath.Join(procRoot, strconv.Itoa(int(pid)), "comm")

	var file *os.File
	err := errors.New("iteration start")
	for i := 0; err != nil && i < 30; i++ {
		file, err = os.Open(filePath)
		if err != nil {
			time.Sleep(1 * time.Millisecond)
		}
	}

	buf := taskCommLenBufferPool.Get().(*[]byte)
	defer taskCommLenBufferPool.Put(buf)
	n, err := file.Read(*buf)
	if err != nil {
		// short living process can hit here, or slow start of another process.
		return false
	}
	return bytes.Equal(bytes.TrimSpace((*buf)[:n]), buildKitProcessName)
}

func isContainerdTmpMount(path string) bool {
	return strings.Contains(path, "tmpmounts/containerd-mount")
}

// getUID() return a key of length 5 as the kernel uprobe registration path is limited to a length of 64
// ebpf-manager/utils.go:GenerateEventName() MaxEventNameLen = 64
// MAX_EVENT_NAME_LEN (linux/kernel/trace/trace.h)
//
// Length 5 is arbitrary value as the full string of the eventName format is
//
//	fmt.Sprintf("%s_%.*s_%s_%s", probeType, maxFuncNameLen, functionName, UID, attachPIDstr)
//
// functionName is variable but with a minimum guarantee of 10 chars
func getUID(lib utils.PathIdentifier) string {
	return lib.Key()[:5]
}

func (ua *UprobeAttacher) attachToBinary(fpath utils.FilePath, matchingRules []*AttachRules) error {
	// TODO: Retrieve this information once and reuse it
	if isBuildKit(ua.config.ProcRoot, fpath.PID) {
		return fmt.Errorf("process %d is buildkitd, skipping", fpath.PID)
	} else if isContainerdTmpMount(fpath.HostPath) {
		return fmt.Errorf("path %s from process %d is tempmount of containerd, skipping", fpath.HostPath, fpath.PID)
	}

	elfFile, err := elf.Open(fpath.HostPath)
	if err != nil {
		return err
	}
	defer elfFile.Close()

	// This only allows amd64 and arm64 and not the 32-bit variants, but that
	// is fine since we don't monitor 32-bit applications at all in the shared
	// library watcher since compat syscalls aren't supported by the syscall
	// trace points. We do actually monitor 32-bit applications for istio and
	// nodejs monitoring, but our uprobe hooks only properly support 64-bit
	// applications, so there's no harm in rejecting 32-bit applications here.
	arch, err := bininspect.GetArchitecture(elfFile)
	if err != nil {
		return fmt.Errorf("cannot get architecture of %s: %w", fpath.HostPath, err)
	}

	// Ignore foreign architectures.  This can happen when running stuff under
	// qemu-user, for example, and installing a uprobe will lead to segfaults
	// since the foreign instructions will be patched with the native break
	// instruction.
	if string(arch) != runtime.GOARCH {
		return fmt.Errorf("unsupported architecture %s for %s", arch, fpath.HostPath)
	}

	symbolMap, err := ua.getAvailableRequestedSymbols(elfFile)
	if err != nil {
		return fmt.Errorf("cannot get symbols for file %v: %w", fpath, err)
	}

	uid := getUID(fpath.ID)

	for _, selector := range ua.config.ProbeSelectors {
		_, isBestEffort := selector.(*manager.BestEffort)
		for _, origProbeId := range selector.GetProbesIdentificationPairList() {
			newProbeId := manager.ProbeIdentificationPair{
				EBPFFuncName: origProbeId.EBPFFuncName,
				UID:          uid,
			}

			// Ensure that all ID pairs have the same UID
			selector.EditProbeIdentificationPair(origProbeId, newProbeId)

			probe, found := ua.manager.GetProbe(newProbeId)
			if found {
				// We have already probed this process, just ensure it's running and skip it
				if !probe.IsRunning() {
					err := probe.Attach()
					if err != nil {
						return err
					}
				}

				continue
			}

			_, symbol, ok := strings.Cut(newProbeId.EBPFFuncName, "__")
			if !ok {
				continue
			}

			sym, found := symbolMap[symbol]
			if !found {
				if isBestEffort {
					continue
				}
				// This should not happen, as getAvailableRequestedSymbols should have already
				// returned an error if mandatory symbols weren't found. However and for safety,
				// we'll check again and return an error if the symbol is not found.
				return fmt.Errorf("symbol %s not found in %s", symbol, fpath.HostPath)
			}
			manager.SanitizeUprobeAddresses(elfFile, []elf.Symbol{sym})
			offset, err := bininspect.SymbolToOffset(elfFile, sym)
			if err != nil {
				return err
			}

			newProbe := &manager.Probe{
				ProbeIdentificationPair: newProbeId,
				BinaryPath:              fpath.HostPath,
				UprobeOffset:            uint64(offset),
				HookFuncName:            symbol,
			}
			err = ua.manager.AddHook("", newProbe)
			if err != nil {
				return fmt.Errorf("error attaching probe %+v: %w", newProbe, err)
			}

			if ua.onAttachCallback != nil {
				ua.onAttachCallback(newProbe)
			}

			ua.pathToAttachedProbes[fpath.HostPath] = append(ua.pathToAttachedProbes[fpath.HostPath], newProbeId)
		}

		manager, ok := ua.manager.(*manager.Manager)
		if ok {
			if err := selector.RunValidator(manager); err != nil {
				return fmt.Errorf("error validating probes: %w", err)
			}
		}
	}

	return nil
}

func (ua *UprobeAttacher) getAvailableRequestedSymbols(elfFile *elf.File) (map[string]elf.Symbol, error) {
	symbolsSet := make(common.StringSet)
	symbolsSetBestEffort := make(common.StringSet)
	for _, selector := range ua.config.ProbeSelectors {
		_, isBestEffort := selector.(*manager.BestEffort)
		for _, selector := range selector.GetProbesIdentificationPairList() {
			_, symbol, ok := strings.Cut(selector.EBPFFuncName, "__")
			if !ok {
				continue
			}
			if isBestEffort {
				symbolsSetBestEffort[symbol] = struct{}{}
			} else {
				symbolsSet[symbol] = struct{}{}
			}
		}
	}
	symbolMap, err := bininspect.GetAllSymbolsByName(elfFile, symbolsSet)
	if err != nil {
		return nil, err
	}
	/* Best effort to resolve symbols, so we don't care about the error */
	symbolMapBestEffort, _ := bininspect.GetAllSymbolsByName(elfFile, symbolsSetBestEffort)
	maps.Copy(symbolMap, symbolMapBestEffort)

	return symbolMap, nil
}

func (ua *UprobeAttacher) detachFromBinary(fpath utils.FilePath) error {
	for _, probeId := range ua.pathToAttachedProbes[fpath.HostPath] {
		err := ua.manager.DetachHook(probeId)
		if err != nil {
			return fmt.Errorf("error detaching probe %+v: %w", probeId, err)
		}
	}

	return nil
}

func (ua *UprobeAttacher) getLibrariesFromMapsFile(pid int) ([]string, error) {
	mapsPath := filepath.Join(ua.config.ProcRoot, strconv.Itoa(int(pid)), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open maps file at %s: %w", mapsPath, err)
	}
	defer mapsFile.Close()

	scanner := bufio.NewScanner(bufio.NewReader(mapsFile))
	libs := make(map[string]struct{})
	for scanner.Scan() {
		line := scanner.Text()
		cols := strings.Fields(line)
		// ensuring we have exactly 6 elements (skip '(deleted)' entries) in the line, and the 4th element (inode) is
		// not zero (indicates it is a path, and not an anonymous path).
		if len(cols) == 6 && cols[4] != "0" {
			libs[cols[5]] = struct{}{}
		}
	}

	return maps.Keys(libs), nil
}

func (ua *UprobeAttacher) attachToLibrariesOfPID(pid uint32) error {
	registerErrors := make([]error, 0)
	successfulMatches := make([]string, 0)
	libs, err := ua.getLibrariesFromMapsFile(int(pid))
	if err != nil {
		return err
	}
	for _, libpath := range libs {
		err := ua.AttachLibrary(libpath, pid)

		if err == nil {
			successfulMatches = append(successfulMatches, libpath)
		} else if !errors.Is(err, ErrNoMatchingRule) {
			registerErrors = append(registerErrors, err)
		}
	}

	if len(successfulMatches) == 0 {
		if len(registerErrors) == 0 {
			return fmt.Errorf("no rules matched for pid %d", pid)
		}
		return fmt.Errorf("no rules matched for pid %d, errors: %v", pid, registerErrors)
	}
	if len(registerErrors) > 0 {
		return fmt.Errorf("partially hooked (%v), errors while attaching pid %d: %v", successfulMatches, pid, registerErrors)
	}
	return nil
}
