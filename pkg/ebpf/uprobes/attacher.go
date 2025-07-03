// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ExcludeMode defines the different optiont to exclude processes from attachment
type ExcludeMode uint8

const (
	// ExcludeSelf excludes the agent's own PID
	ExcludeSelf ExcludeMode = 1 << iota
	// ExcludeInternal excludes internal DataDog processes
	ExcludeInternal
	// ExcludeBuildkit excludes buildkitd processes
	ExcludeBuildkit
	// ExcludeContainerdTmp excludes containerd tmp mounts
	ExcludeContainerdTmp
)

var (
	// ErrSelfExcluded is returned when the PID is the same as the agent's PID.
	ErrSelfExcluded = errors.New("self-excluded")
	// ErrInternalDDogProcessRejected is returned when the PID is an internal datadog process.
	ErrInternalDDogProcessRejected = errors.New("internal datadog process rejected")
	// ErrNoMatchingRule is returned when no rule matches the shared library path.
	ErrNoMatchingRule = errors.New("no matching rule")
	// regex that defines internal DataDog processes
	internalProcessRegex = regexp.MustCompile("datadog-agent/.*/((process|security|trace|otel)-agent|system-probe|agent)")
)

// AttachTarget defines the target to which we should attach the probes, libraries or executables
type AttachTarget uint8

const (
	// AttachToExecutable attaches to the main executable
	AttachToExecutable AttachTarget = 1 << iota
	// AttachToSharedLibraries attaches to shared libraries
	AttachToSharedLibraries
)

// ProbeOptions is a structure that holds the options for a probe attachment. By default
// these values will be inferred from the probe name, but the user can override them if needed.
type ProbeOptions struct {
	// IsManualReturn indicates that the probe is a manual return probe, which means that the inspector
	// will find the return locations of the function and attach to them instead of using uretprobes.
	IsManualReturn bool

	// Symbol is the symbol name to attach the probe to. This is useful when the symbol name is not a valid
	// C identifier (e.g. Go functions)
	Symbol string
}

// AttachRule defines how to attach a certain set of probes. Uprobes can be attached
// to shared libraries or executables, this structure tells the attacher which ones to
// select and to which targets to do it.
type AttachRule struct {
	// LibraryNameRegex defines which libraries should be matched by this rule
	LibraryNameRegex *regexp.Regexp
	// ExecutableFilter is a function that receives the path of the executable and returns true if it should be matched
	ExecutableFilter func(string, *ProcInfo) bool
	// Targets defines the targets to which we should attach the probes, shared libraries and/or executables
	Targets AttachTarget
	// ProbesSelectors defines which probes should be attached and how should we validate
	// the attachment (e.g., whether we need all probes active or just one of them, or in a best-effort basis)
	ProbesSelector []manager.ProbesSelector
	// ProbeOptionsOverride allows the user to override the options for a probe that are inferred from the name
	// of the probe. This way the user can set options such as manual return detection or symbol names for probes
	// whose names aren't valid C identifiers.
	ProbeOptionsOverride map[string]ProbeOptions
}

// canTarget returns true if the rule matches the given AttachTarget
func (r *AttachRule) canTarget(target AttachTarget) bool {
	return r.Targets&target != 0
}

func (r *AttachRule) matchesLibrary(path string) bool {
	return r.canTarget(AttachToSharedLibraries) && r.LibraryNameRegex != nil && r.LibraryNameRegex.MatchString(path)
}

func (r *AttachRule) matchesExecutable(path string, procInfo *ProcInfo) bool {
	return r.canTarget(AttachToExecutable) && (r.ExecutableFilter == nil || r.ExecutableFilter(path, procInfo))
}

// getProbeOptions returns the options for a given probe, checking if we have specific overrides
// in this rule and, if not, using the options inferred from the probe name.
func (r *AttachRule) getProbeOptions(probeID manager.ProbeIdentificationPair) (ProbeOptions, error) {
	if r.ProbeOptionsOverride != nil {
		if options, ok := r.ProbeOptionsOverride[probeID.EBPFFuncName]; ok {
			return options, nil
		}
	}

	symbol, isManualReturn, err := parseSymbolFromEBPFProbeName(probeID.EBPFFuncName)
	if err != nil {
		return ProbeOptions{}, err
	}

	return ProbeOptions{
		Symbol:         symbol,
		IsManualReturn: isManualReturn,
	}, nil
}

// Validate checks whether the rule is valid and compatible with the given attacher config, returns nil if it is, an error message otherwise
func (r *AttachRule) Validate(attacherConfig *AttacherConfig) error {
	var result error

	if r.Targets == 0 {
		result = multierror.Append(result, errors.New("no targets specified"))
	}

	if r.canTarget(AttachToSharedLibraries) {
		if r.LibraryNameRegex == nil {
			result = multierror.Append(result, errors.New("no library name regex specified"))
		} else {
			// A sanity check to ensure that the library name regex matches at least one library in the libset.
			// The best way to do this would be to check that the language generated by the "regex" used in the eBPF
			// program is a subset of the language generated by the regexes in the libset, but that's a hard problem,
			// so we do a simple check: either the regex matches a library name in the libset, or the regex contains
			// a substring that matches a library name in the libset.
			matchesAtLeastOneLib := false
		outer:
			for _, libset := range attacherConfig.SharedLibsLibsets {
				suffixes := sharedlibraries.LibsetToLibSuffixes[libset]
				for _, libSuffix := range suffixes {
					libSuffixWithExt := libSuffix + ".so"
					if r.LibraryNameRegex.MatchString(libSuffixWithExt) || strings.Contains(r.LibraryNameRegex.String(), libSuffix) {
						matchesAtLeastOneLib = true
						break outer
					}
				}
			}

			if !matchesAtLeastOneLib {
				result = multierror.Append(result, fmt.Errorf("library name regex %s does not match any library in libsets [%v]", r.LibraryNameRegex, attacherConfig.SharedLibsLibsets))
			}
		}
	}

	for _, selector := range r.ProbesSelector {
		for _, probeID := range selector.GetProbesIdentificationPairList() {
			_, err := r.getProbeOptions(probeID)
			if err != nil {
				result = multierror.Append(result, fmt.Errorf("cannot get options for probe %s: %w", probeID.EBPFFuncName, err))
			}
		}
	}

	return result
}

// ProcessMonitor is an interface that allows subscribing to process start and exit events
type ProcessMonitor interface {
	// SubscribeExec subscribes to process start events, with a callback that
	// receives the PID of the process. Returns a function that can be called to
	// unsubscribe from the event.
	SubscribeExec(func(uint32)) func()

	// SubscribeExit subscribes to process exit events, with a callback that
	// receives the PID of the process. Returns a function that can be called to
	// unsubscribe from the event.
	SubscribeExit(func(uint32)) func()
}

// AttacherConfig defines the configuration for the attacher
type AttacherConfig struct {
	// Rules defines a series of rules that tell the attacher how to attach the probes
	Rules []*AttachRule

	// ScanProcessesInterval defines the interval at which we scan for terminated processes and new processes we haven't seen
	ScanProcessesInterval time.Duration

	// EnablePeriodicScanNewProcesses defines whether the attacher should scan for new processes periodically (with ScanProcessesInterval)
	EnablePeriodicScanNewProcesses bool

	// ProcRoot is the root directory of the proc filesystem
	ProcRoot string

	// ExcludeTargets defines the targets that should be excluded from the attacher
	ExcludeTargets ExcludeMode

	// EbpfConfig is the configuration for the eBPF program
	EbpfConfig *ebpf.Config

	// PerformInitialScan defines if the attacher should perform an initial scan of the processes before starting the monitor
	// Note that if processMonitor is being used (i.e., rules are targeting executables), the ProcessMonitor itself
	// will perform an initial scan in its Initialize method.
	PerformInitialScan bool

	// EnableDetailedLogging makes the attacher log why it's attaching or not attaching to a process
	// This is useful for debugging purposes, do not enable in production.
	EnableDetailedLogging bool

	// If shared libraries tracing is enabled, this is the list of library sets to use to filter the events
	// from the shared libraries program.
	SharedLibsLibsets []sharedlibraries.Libset

	// OnSyncCallback is an optional function that gets called when the attacher performs a sync. Receives as an argument
	// the set of alive PIDs in the system.
	// This function will not be called in the same goroutine as the attacher code, so it will not block process creation/deletion
	// events from happening. However, if it takes too long it can delay the next sync. In any case, synchronizations are
	// usually performed every 30 seconds by default (ScanProcessesInterval), so it shouldn't be a problem.
	OnSyncCallback func(map[uint32]struct{})

	// MaxPeriodicScansPerProcess defines the maximum number of periodic scans we will perform for a given PID
	// when EnablePeriodicScanNewProcesses is true. Useful to avoid re-scanning processes that have already
	// been scanned, specially when shared libraries are being traced as scanning the maps file can be expensive.
	MaxPeriodicScansPerProcess int
}

// SetDefaults configures the AttacherConfig with default values for those fields for which the compiler
// defaults are not enough
func (ac *AttacherConfig) SetDefaults() {
	if ac.ScanProcessesInterval == 0 {
		ac.ScanProcessesInterval = 30 * time.Second
	}

	if ac.ProcRoot == "" {
		ac.ProcRoot = kernel.HostProc()
	}

	if ac.EbpfConfig == nil {
		ac.EbpfConfig = ebpf.NewConfig()
	}

	if ac.MaxPeriodicScansPerProcess == 0 {
		// 2 seems a reasonable default, as we will give time (1 minute with the default interval)
		// to the process to start and for any environmental errors to stop affecting the process
		// and allow it to load the shared libraries we might be interested in.
		ac.MaxPeriodicScansPerProcess = 2
	}
}

// Validate checks whether the configuration is valid, returns nil if it is, an error message otherwise
func (ac *AttacherConfig) Validate() error {
	var err error

	if ac.EbpfConfig == nil {
		err = multierror.Append(err, errors.New("missing ebpf config"))
	}

	if ac.ProcRoot == "" {
		err = multierror.Append(err, errors.New("missing proc root"))
	}

	targetsSharedLibs := false
	for _, rule := range ac.Rules {
		validateErr := rule.Validate(ac)
		if validateErr != nil {
			err = multierror.Append(err, validateErr)
		}

		targetsSharedLibs = targetsSharedLibs || rule.canTarget(AttachToSharedLibraries)
	}

	if targetsSharedLibs {
		for _, libset := range ac.SharedLibsLibsets {
			if !sharedlibraries.IsLibsetValid(libset) {
				err = multierror.Append(err, fmt.Errorf("invalid libset %s", libset))
			}
		}
	}

	return err
}

// ProbeManager is an interface that defines the methods that a Manager implements,
// so that we can replace it in tests for a mock object
type ProbeManager interface {
	// AddHook adds a hook to the manager with the given UID and probe
	AddHook(UID string, probe *manager.Probe) error

	// DetachHook detaches the hook with the ID pair
	DetachHook(manager.ProbeIdentificationPair) error

	// GetProbe returns the probe with the given ID pair, and a boolean indicating if it was found
	GetProbe(manager.ProbeIdentificationPair) (*manager.Probe, bool)
}

// FileRegistry is an interface that defines the methods that a FileRegistry implements, so that we can replace it in tests for a mock object
type FileRegistry interface {
	// Register registers a file path to be tracked by the attacher for the given PID. The registry will call the activationCB when the file is opened
	// the first time, and the deactivationCB when the file is closed. If the file is already registered, the alreadyRegistered callback
	// will be called instead of the activationCB.
	Register(namespacedPath string, pid uint32, activationCB, deactivationCB, alreadyRegistered utils.Callback) error

	// Unregister unregisters a file path from the attacher. The deactivation callback will be called for all
	// files that were registered with the given PID and aren't used anymore.
	Unregister(uint32) error

	// Clear clears the registry, removing all registered files
	Clear()

	// GetRegisteredProcesses returns a map of all the processes that are currently registered in the registry
	GetRegisteredProcesses() map[uint32]struct{}

	// Log is a function that gets called periodically to log the state of the registry
	Log()
}

// AttachCallback is a callback that is called whenever a probe is attached successfully
type AttachCallback func(*manager.Probe, *utils.FilePath)

// NopOnAttachCallback is a callback that indicates that no action should be taken for the callback
var NopOnAttachCallback AttachCallback

// UprobeAttacher is a struct that handles the attachment of uprobes to processes and libraries
type UprobeAttacher struct {
	// name contains the name of this attacher for identification
	name string

	// done is a channel to signal the attacher to stop
	done chan struct{}

	// wg is a wait group to wait for the attacher to stop
	wg sync.WaitGroup

	// config holds the configuration of the attacher. Not a pointer as we want
	// a copy of the configuration so that the user cannot change it, as we have
	// certain cached values that we have no way to invalidate if the config
	// changes after the attacher is created
	config AttacherConfig

	// fileRegistry is used to keep track of the files we are attached to, and attach only once to each file
	fileRegistry FileRegistry

	// manager is used to manage the eBPF probes (attach/detach to processes)
	manager ProbeManager

	// inspector is used  extract the metadata from the binaries
	inspector BinaryInspector

	// fileIDToAttachedProbes maps a filesystem path to the probes attached to it.
	// Used to detach them once the path is no longer used.
	fileIDToAttachedProbes map[utils.PathIdentifier][]manager.ProbeIdentificationPair

	// onAttachCallback is a callback that is called whenever a probe is attached
	onAttachCallback AttachCallback

	// soWatcher is the program that launches events whenever shared libraries are
	// opened
	soWatcher *sharedlibraries.EbpfProgram

	// handlesLibrariesCached is a cache for the handlesLibraries function, avoiding
	// recomputation every time
	handlesLibrariesCached *bool

	// handlesExecutablesCached is a cache for the handlesExecutables function, avoiding
	// recomputation every time
	handlesExecutablesCached *bool

	// processMonitor is the process monitor that we use to subscribe to process start and exit events
	processMonitor ProcessMonitor

	// scansPerPid is a map of PIDs to the number of times we have scanned them, to avoid re-scanning them
	// too many times when EnablePeriodicScanNewProcesses is true
	scansPerPid map[uint32]int

	// attachLimiter is used to limit the number of times we log warnings about attachment errors
	attachLimiter *log.Limit
}

// NewUprobeAttacher creates a new UprobeAttacher. Receives as arguments
//   - The name of the attacher
//   - The configuration. Note that the config is copied, not referenced. The attacher caches some values
//     that depend on the configuration, so any changes to the configuration after the
//     attacher would make those caches incoherent. This way we ensure that the attacher is always consistent with the configuration it was created with.
//   - The eBPF probe manager (ebpf.Manager usually)
//   - A callback to be called whenever a probe is attached (optional, can be nil)
//   - The binary inspector to be used (e.g., while we usually want NativeBinaryInspector here,
//     we might want the GoBinaryInspector to attach to Go functions in a different
//     way).
//   - The process monitor to be used to subscribe to process start and exit events. The lifecycle of the process monitor is managed by the caller, the attacher
//     will not stop the monitor when it stops.
func NewUprobeAttacher(moduleName, name string, config AttacherConfig, mgr ProbeManager, onAttachCallback AttachCallback, inspector BinaryInspector, processMonitor ProcessMonitor) (*UprobeAttacher, error) {
	config.SetDefaults()

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid attacher configuration: %w", err)
	}

	ua := &UprobeAttacher{
		name:                   name,
		config:                 config,
		fileRegistry:           utils.NewFileRegistry(moduleName, name),
		manager:                mgr,
		onAttachCallback:       onAttachCallback,
		fileIDToAttachedProbes: make(map[utils.PathIdentifier][]manager.ProbeIdentificationPair),
		done:                   make(chan struct{}),
		inspector:              inspector,
		processMonitor:         processMonitor,
		scansPerPid:            make(map[uint32]int),
		attachLimiter:          log.NewLogLimit(10, 10*time.Minute),
	}

	utils.AddAttacher(moduleName, name, ua)

	return ua, nil
}

// handlesLibraries returns whether the attacher has rules configured to attach to shared libraries.
// It caches the result to avoid recalculating it every time we are attaching to a PID.
func (ua *UprobeAttacher) handlesLibraries() bool {
	if ua.handlesLibrariesCached != nil {
		return *ua.handlesLibrariesCached
	}

	result := false
	for _, rule := range ua.config.Rules {
		if rule.canTarget(AttachToSharedLibraries) {
			result = true
			break
		}
	}
	ua.handlesLibrariesCached = &result
	return result
}

// handlesExecutables returns whether the attacher has rules configured to attach to executables directly
// It caches the result to avoid recalculating it every time we are attaching to a PID.
func (ua *UprobeAttacher) handlesExecutables() bool {
	if ua.handlesExecutablesCached != nil {
		return *ua.handlesExecutablesCached
	}

	result := false
	for _, rule := range ua.config.Rules {
		if rule.canTarget(AttachToExecutable) {
			result = true
			break
		}
	}
	ua.handlesExecutablesCached = &result
	return result
}

// Start starts the attacher, attaching to the processes and libraries as needed
func (ua *UprobeAttacher) Start() error {
	var cleanupExec, cleanupExit func()
	var cleanupSharedLibs func()

	if ua.handlesExecutables() {
		cleanupExec = ua.processMonitor.SubscribeExec(ua.handleProcessStart)
	}
	// We always want to track process deletions, to avoid memory leaks
	cleanupExit = ua.processMonitor.SubscribeExit(ua.handleProcessExit)

	if ua.handlesLibraries() {
		if !sharedlibraries.IsSupported(ua.config.EbpfConfig) {
			return errors.New("shared libraries tracing not supported for this platform")
		}

		ua.soWatcher = sharedlibraries.GetEBPFProgram(ua.config.EbpfConfig)

		err := ua.soWatcher.InitWithLibsets(ua.config.SharedLibsLibsets...)
		if err != nil {
			return fmt.Errorf("error initializing shared library program: %w", err)
		}

		cleanupSharedLibs, err = ua.soWatcher.Subscribe(ua.handleLibraryOpen, ua.config.SharedLibsLibsets...)
		if err != nil {
			return fmt.Errorf("error subscribing to shared libraries events: %w", err)
		}
	}

	if ua.config.PerformInitialScan {
		// Initial scan only looks at existing processes, and as it's the first scan
		// we don't have to track deletions
		err := ua.Sync(true, false)
		if err != nil {
			return fmt.Errorf("error during initial scan: %w", err)
		}
	}

	ua.wg.Add(1)
	go func() {
		processSync := time.NewTicker(ua.config.ScanProcessesInterval)

		defer func() {
			processSync.Stop()
			if cleanupExec != nil {
				cleanupExec()
			}
			cleanupExit()
			ua.fileRegistry.Clear()
			if ua.soWatcher != nil {
				ua.soWatcher.Stop()
			}
			if cleanupSharedLibs != nil {
				cleanupSharedLibs()
			}
			ua.wg.Done()
			log.Infof("uprobe attacher %s stopped", ua.name)
		}()

		for {
			select {
			case <-ua.done:
				return
			case <-processSync.C:
				// We always track process deletions in the scan, to avoid memory leaks.
				_ = ua.Sync(ua.config.EnablePeriodicScanNewProcesses, true)

				// Periodically log the state of the registry
				ua.fileRegistry.Log()
			}
		}
	}()
	log.Infof("uprobe attacher %s started", ua.name)

	return nil
}

// Sync scans the proc filesystem for new processes and detaches from terminated ones
func (ua *UprobeAttacher) Sync(trackCreations, trackDeletions bool) error {
	if !trackDeletions && !trackCreations && ua.config.OnSyncCallback == nil {
		return nil // Nothing to do
	}

	var deletionCandidates map[uint32]struct{}
	if trackDeletions {
		deletionCandidates = ua.fileRegistry.GetRegisteredProcesses()
	}
	thisPID, err := kernel.RootNSPID()
	if err != nil {
		return err
	}

	alivePIDs := make(map[uint32]struct{})
	_ = kernel.WithAllProcs(ua.config.ProcRoot, func(p int) error {
		if p == thisPID { // don't scan ourselves
			return nil
		}

		pid := uint32(p)

		alivePIDs[pid] = struct{}{}

		if trackDeletions {
			if _, ok := deletionCandidates[pid]; ok {
				// We have previously hooked into this process and it remains active,
				// so we remove it from the deletionCandidates list, and move on to the next PID
				delete(deletionCandidates, pid)
				return nil
			}
		}

		if trackCreations && ua.scansPerPid[pid] < ua.config.MaxPeriodicScansPerProcess {
			// This is a new PID so we attempt to attach SSL probes to it
			ua.scansPerPid[pid]++
			err := ua.AttachPID(pid)
			if err == nil {
				if ua.config.EnableDetailedLogging {
					log.Debugf("uprobe attacher %s attached to process %d via periodic scan", ua.name, pid)
				}

				// Set the number of scans to the maximum so we don't try to scan it again
				ua.scansPerPid[pid] = ua.config.MaxPeriodicScansPerProcess
			} else {
				if ua.shouldLogRegistryError(err) {
					log.Warnf("could not attach to process %d: %v", pid, err)
				}
			}
		}
		return nil
	})

	// Clean up the scansPerPid map, removing all PIDs that are no longer alive
	for pid := range ua.scansPerPid {
		if _, ok := alivePIDs[pid]; !ok {
			delete(ua.scansPerPid, pid)
		}
	}

	if trackDeletions {
		// At this point all entries from deletionCandidates are no longer alive, so
		// we should detach our SSL probes from them
		for pid := range deletionCandidates {
			ua.handleProcessExit(pid)
		}
	}

	if ua.config.OnSyncCallback != nil {
		ua.config.OnSyncCallback(alivePIDs)
	}

	return nil
}

// Stop stops the attacher
func (ua *UprobeAttacher) Stop() {
	close(ua.done)
	ua.wg.Wait()
}

func (ua *UprobeAttacher) shouldLogRegistryError(err error) bool {
	if err == nil {
		return false
	}

	// Always log all errors if detailed logging is enabled
	if ua.config.EnableDetailedLogging {
		return true
	}

	var unknownErr *utils.UnknownAttachmentError
	if errors.As(err, &unknownErr) {
		return ua.attachLimiter.ShouldLog()
	}
	return false
}

// handleProcessStart is called when a new process is started, wraps AttachPIDWithOptions but ignoring the error
// for API compatibility with processMonitor
func (ua *UprobeAttacher) handleProcessStart(pid uint32) {
	err := ua.AttachPIDWithOptions(pid, false) // Do not try to attach to libraries on process start, it hasn't loaded them yet
	if ua.shouldLogRegistryError(err) {
		log.Warnf("could not attach to process %d: %v", pid, err)
	}
}

// handleProcessExit is called when a process finishes, wraps DetachPID but ignoring the error
// for API compatibility with processMonitor
func (ua *UprobeAttacher) handleProcessExit(pid uint32) {
	_ = ua.DetachPID(pid)
}

func (ua *UprobeAttacher) handleLibraryOpen(libpath sharedlibraries.LibPath) {
	path := sharedlibraries.ToBytes(&libpath)

	err := ua.AttachLibrary(string(path), libpath.Pid)
	if ua.shouldLogRegistryError(err) {
		log.Warnf("could not attach to library %s (PID %d): %v", path, libpath.Pid, err)
	}
}

func (ua *UprobeAttacher) buildRegisterCallbacks(matchingRules []*AttachRule, procInfo *ProcInfo) (func(utils.FilePath) error, func(utils.FilePath) error) {
	registerCB := func(p utils.FilePath) error {
		err := ua.attachToBinary(p, matchingRules, procInfo)
		if ua.config.EnableDetailedLogging {
			log.Debugf("uprobes: attaching to %s (PID %d): err=%v", p.HostPath, procInfo.PID, err)
		}
		return err
	}
	unregisterCB := func(p utils.FilePath) error {
		err := ua.detachFromBinary(p)
		if ua.config.EnableDetailedLogging {
			log.Debugf("uprobes: detaching from %s (PID %d): err=%v", p.HostPath, p.PID, err)
		}
		return err
	}

	return registerCB, unregisterCB
}

// AttachLibrary attaches the probes to the given library, opened by a given PID
func (ua *UprobeAttacher) AttachLibrary(path string, pid uint32) error {
	if (ua.config.ExcludeTargets&ExcludeSelf) != 0 && int(pid) == os.Getpid() {
		return ErrSelfExcluded
	}

	matchingRules := ua.getRulesForLibrary(path)
	if len(matchingRules) == 0 {
		return ErrNoMatchingRule
	}

	registerCB, unregisterCB := ua.buildRegisterCallbacks(matchingRules, NewProcInfo(ua.config.ProcRoot, pid))

	return ua.fileRegistry.Register(path, pid, registerCB, unregisterCB, utils.IgnoreCB)
}

// getRulesForLibrary returns the rules that match the given library path
func (ua *UprobeAttacher) getRulesForLibrary(path string) []*AttachRule {
	var matchedRules []*AttachRule

	for _, rule := range ua.config.Rules {
		if rule.matchesLibrary(path) {
			matchedRules = append(matchedRules, rule)
		}
	}
	return matchedRules
}

// getRulesForExecutable returns the rules that match the given executable
func (ua *UprobeAttacher) getRulesForExecutable(path string, procInfo *ProcInfo) []*AttachRule {
	var matchedRules []*AttachRule

	for _, rule := range ua.config.Rules {
		if rule.matchesExecutable(path, procInfo) {
			matchedRules = append(matchedRules, rule)
		}
	}
	return matchedRules
}

// getExecutablePath resolves the executable of the given PID looking in procfs.
// Will return an error if the path cannot be resolved
func (ua *UprobeAttacher) getExecutablePath(pid uint32) (string, error) {
	pidAsStr := strconv.FormatUint(uint64(pid), 10)
	exePath := filepath.Join(ua.config.ProcRoot, pidAsStr, "exe")

	return os.Readlink(exePath)
}

const optionAttachToLibs = true

// AttachPID attaches the corresponding probes to a given pid
func (ua *UprobeAttacher) AttachPID(pid uint32) error {
	return ua.AttachPIDWithOptions(pid, optionAttachToLibs)
}

// AttachPIDWithOptions attaches the corresponding probes to a given pid
func (ua *UprobeAttacher) AttachPIDWithOptions(pid uint32, attachToLibs bool) error {
	if (ua.config.ExcludeTargets&ExcludeSelf) != 0 && int(pid) == os.Getpid() {
		return ErrSelfExcluded
	}

	procInfo := NewProcInfo(ua.config.ProcRoot, pid)

	// Only compute the binary path if we are going to need it. It's better to do these two checks
	// (which are cheap, the handlesExecutables function is cached) than to do the syscall
	// every time
	var binPath string
	var err error
	if ua.handlesExecutables() || (ua.config.ExcludeTargets&ExcludeInternal) != 0 {
		binPath, err = procInfo.Exe()
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return utils.NewUnknownAttachmentError(err)
			}
			return err
		}
	}

	if (ua.config.ExcludeTargets&ExcludeInternal) != 0 && internalProcessRegex.MatchString(binPath) {
		return ErrInternalDDogProcessRejected
	}

	if ua.handlesExecutables() {
		matchingRules := ua.getRulesForExecutable(binPath, procInfo)
		if len(matchingRules) != 0 {
			registerCB, unregisterCB := ua.buildRegisterCallbacks(matchingRules, procInfo)
			err = ua.fileRegistry.Register(binPath, pid, registerCB, unregisterCB, utils.IgnoreCB)
			// Do not return in case of error, as we still might want to attach to libraries
		}
	}

	if attachToLibs && ua.handlesLibraries() {
		err = errors.Join(err, ua.attachToLibrariesOfPID(pid))
	}

	return err
}

// DetachPID detaches the uprobes attached to a PID
func (ua *UprobeAttacher) DetachPID(pid uint32) error {
	return ua.fileRegistry.Unregister(pid)
}

const buildKitProcessName = "buildkitd"

func isBuildKit(procInfo *ProcInfo) bool {
	comm, err := procInfo.Comm()
	if err != nil {
		return false
	}
	return strings.HasPrefix(comm, buildKitProcessName)
}

func isContainerdTmpMount(path string) bool {
	return strings.Contains(path, "tmpmounts/containerd-mount") || strings.Contains(path, "/tmp/ctd-volume")
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

func parseSymbolFromEBPFProbeName(probeName string) (symbol string, isManualReturn bool, err error) {
	parts := strings.Split(probeName, "__")
	if len(parts) < 2 {
		err = fmt.Errorf("invalid probe name %s, no double underscore (__) separating probe type and function name", probeName)
		return
	}

	symbol = parts[1]
	if len(parts) > 2 {
		if parts[2] == "return" {
			isManualReturn = true
		} else {
			err = fmt.Errorf("invalid probe name %s, unexpected third part %s. Format should be probeType__funcName[__return]", probeName, parts[2])
			return
		}
	}

	return
}

// attachToBinary attaches the probes to the given binary. Important: it does not perform any cleanup on failure.
// This is to match the behavior of the FileRegistry, which will call the deactivation callback on failure of the registration
// callback.
func (ua *UprobeAttacher) attachToBinary(fpath utils.FilePath, matchingRules []*AttachRule, procInfo *ProcInfo) error {
	if ua.config.ExcludeTargets&ExcludeBuildkit != 0 && isBuildKit(procInfo) {
		return fmt.Errorf("%w: process %d is buildkitd, skipping", utils.ErrEnvironment, fpath.PID)
	} else if ua.config.ExcludeTargets&ExcludeContainerdTmp != 0 && isContainerdTmpMount(fpath.HostPath) {
		return fmt.Errorf("%w: path %s from process %d is tempmount of containerd, skipping", utils.ErrEnvironment, fpath.HostPath, fpath.PID)
	}

	symbolsToRequest, err := ua.computeSymbolsToRequest(matchingRules)
	if err != nil {
		return utils.NewUnknownAttachmentError(fmt.Errorf("error computing symbols to request for rules %+v: %w", matchingRules, err))
	}

	inspectResult, err := ua.inspector.Inspect(fpath, symbolsToRequest)
	if err != nil {
		// Not wrapping this one in an UnknownAttachmentError as it can happen if we're trying to
		// attach to a process that just doesn't have the symbols we're looking for.
		return fmt.Errorf("error inspecting %s: %w", fpath.HostPath, err)
	}

	uid := getUID(fpath.ID)

	for _, rule := range matchingRules {
		for _, selector := range rule.ProbesSelector {
			err = ua.attachProbeSelector(selector, fpath, uid, rule, inspectResult)
			if err != nil {
				// At this point we have done enough validation so that the
				// probe attachment should work, if it doesn't it's an issue we
				// don't expect and we want to know about it.
				return utils.NewUnknownAttachmentError(err)
			}
		}
	}

	return nil
}

func (ua *UprobeAttacher) attachProbeSelector(selector manager.ProbesSelector, fpath utils.FilePath, fpathUID string, rule *AttachRule, inspectResult map[string]bininspect.FunctionMetadata) error {
	_, isBestEffort := selector.(*manager.BestEffort)

	for _, probeID := range selector.GetProbesIdentificationPairList() {
		probeOpts, err := rule.getProbeOptions(probeID)
		if err != nil {
			return fmt.Errorf("error parsing probe name %s: %w", probeID.EBPFFuncName, err)
		}

		data, found := inspectResult[probeOpts.Symbol]
		if !found {
			if isBestEffort {
				return nil
			}
			// This should not happen, as Inspect should have already
			// returned an error if mandatory symbols weren't found.
			// However and for safety, we'll check again and return an
			// error if the symbol is not found.
			return fmt.Errorf("symbol %s not found in %s", probeOpts.Symbol, fpath.HostPath)
		}

		var locationsToAttach []uint64
		var probeTypeCode string // to make unique UIDs between return/non-return probes
		if probeOpts.IsManualReturn {
			locationsToAttach = data.ReturnLocations
			probeTypeCode = "r"
		} else {
			locationsToAttach = []uint64{data.EntryLocation}
			probeTypeCode = "d"
		}

		for i, location := range locationsToAttach {
			newProbeID := manager.ProbeIdentificationPair{
				EBPFFuncName: probeID.EBPFFuncName,
				UID:          fmt.Sprintf("%s%s%d", fpathUID, probeTypeCode, i), // Make UID unique even if we have multiple locations
			}

			probe, found := ua.manager.GetProbe(newProbeID)
			if found {
				// We have already probed this process, just ensure it's running and skip it
				if !probe.IsRunning() {
					err := probe.Attach()
					if err != nil {
						return fmt.Errorf("cannot attach running probe %v: %w", newProbeID, err)
					}
				}
				if ua.config.EnableDetailedLogging {
					log.Debugf("Probe %v already attached to %s", newProbeID, fpath.HostPath)
				}
				continue
			}

			newProbe := &manager.Probe{
				ProbeIdentificationPair: newProbeID,
				BinaryPath:              fpath.HostPath,
				UprobeOffset:            location,
				HookFuncName:            probeOpts.Symbol,
			}
			err = ua.manager.AddHook("", newProbe)
			if err != nil {
				return fmt.Errorf("error attaching probe %+v: %w", newProbe, err)
			}

			ebpf.AddProgramNameMapping(newProbe.ID(), newProbe.EBPFFuncName, ua.name)
			ua.fileIDToAttachedProbes[fpath.ID] = append(ua.fileIDToAttachedProbes[fpath.ID], newProbeID)

			if ua.onAttachCallback != nil {
				ua.onAttachCallback(newProbe, &fpath)
			}

			// Update the probe IDs with the new UID, so that the validator can find them
			// correctly (we're changing UIDs every time)
			selector.EditProbeIdentificationPair(probeID, newProbeID)

			if ua.config.EnableDetailedLogging {
				log.Debugf("Attached probe %v to %s (PID %d)", newProbeID, fpath.HostPath, fpath.PID)
			}
		}
	}

	manager, ok := ua.manager.(*manager.Manager)
	if ok {
		if err := selector.RunValidator(manager); err != nil {
			return fmt.Errorf("error validating probes: %w", err)
		}
	}

	return nil
}

func (ua *UprobeAttacher) computeSymbolsToRequest(rules []*AttachRule) ([]SymbolRequest, error) {
	var requests []SymbolRequest
	for _, rule := range rules {
		for _, selector := range rule.ProbesSelector {
			_, isBestEffort := selector.(*manager.BestEffort)
			for _, selector := range selector.GetProbesIdentificationPairList() {
				opts, err := rule.getProbeOptions(selector)
				if err != nil {
					return nil, fmt.Errorf("error parsing probe name %s: %w", selector.EBPFFuncName, err)
				}

				requests = append(requests, SymbolRequest{
					Name:                   opts.Symbol,
					IncludeReturnLocations: opts.IsManualReturn,
					BestEffort:             isBestEffort,
				})
			}
		}
	}

	return requests, nil
}

func (ua *UprobeAttacher) detachFromBinary(fpath utils.FilePath) error {
	for _, probeID := range ua.fileIDToAttachedProbes[fpath.ID] {
		err := ua.manager.DetachHook(probeID)
		if err != nil {
			return fmt.Errorf("error detaching probe %+v: %w", probeID, err)
		}
	}

	delete(ua.fileIDToAttachedProbes, fpath.ID)
	ua.inspector.Cleanup(fpath)

	return nil
}

func (ua *UprobeAttacher) getLibrariesFromMapsFile(pid int) ([]string, error) {
	mapsPath := filepath.Join(ua.config.ProcRoot, strconv.Itoa(pid), "maps")
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
		return utils.NewUnknownAttachmentError(err)
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
			return nil // No libraries found to attach
		}
		return utils.NewUnknownAttachmentError(fmt.Errorf("no rules matched for pid %d, errors: %v", pid, registerErrors))
	}
	if len(registerErrors) > 0 {
		return utils.NewUnknownAttachmentError(fmt.Errorf("partially hooked (%v), errors while attaching pid %d: %v", successfulMatches, pid, registerErrors))
	}
	return nil
}
