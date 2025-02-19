// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"debug/dwarf"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/rlimit"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diconfig"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

type ProcessState int

const (
	StateNew ProcessState = iota
	StateAnalyzing
	StateRunning
	StateExited
)

func (s ProcessState) String() string {
	switch s {
	case StateNew:
		return "NEW"
	case StateAnalyzing:
		return "ANALYZING"
	case StateRunning:
		return "RUNNING"
	case StateExited:
		return "EXITED"
	default:
		return "UNKNOWN"
	}
}

type ProcessInfo struct {
	PID        int
	BinaryPath string
	ParentPID  int
	State      ProcessState
	Children   []*ProcessInfo
	StartTime  time.Time
	Analyzed   bool
}

type ProcessTracker struct {
	t                *testing.T
	mu               sync.RWMutex
	processes        map[int]*ProcessInfo
	mainPID          int
	stopChan         chan struct{}
	analyzedBinaries map[string]bool
	analyzedPIDs     map[int]bool
	done             chan struct{}
}

type ProbeManager struct {
	t               *testing.T
	installedProbes sync.Map // maps pid -> map[string]struct{}
	dataReceived    sync.Map // maps pid -> map[string]bool
	mu              sync.Mutex
}

type BinaryInfo struct {
	path     string
	hasDebug bool
}

type FunctionInfo struct {
	PackageName  string
	FunctionName string
	FullName     string
	ProbeId      string
}

func NewFunctionInfo(packageName, functionName, fullName string) FunctionInfo {
	return FunctionInfo{
		PackageName:  packageName,
		FunctionName: functionName,
		FullName:     fullName,
		ProbeId:      uuid.NewString(),
	}
}

type rcConfig struct {
	ID        string
	Version   int
	ProbeType string `json:"type"`
	Language  string
	Where     struct {
		TypeName   string `json:"typeName"`
		MethodName string `json:"methodName"`
		SourceFile string
		Lines      []string
	}
	Tags            []string
	Template        string
	CaptureSnapshot bool
	EvaluatedAt     string
	Capture         struct {
		MaxReferenceDepth int `json:"maxReferenceDepth"`
		MaxFieldCount     int `json:"maxFieldCount"`
	}
}

type ConfigAccumulator struct {
	configs map[string]map[string]rcConfig
	tmpl    *template.Template
	mu      sync.RWMutex
}

type RepoInfo struct {
	Packages   map[string]bool // Package names found in repo
	RepoPath   string          // Path to the repo
	CommitHash string          // Current commit hash (optional)
}

type explorationEventOutputTestWriter struct {
	t              *testing.T
	expectedResult map[string]*ditypes.CapturedValue
}

func (e *explorationEventOutputTestWriter) Write(p []byte) (n int, err error) {
	var snapshot ditypes.SnapshotUpload
	if err := json.Unmarshal(p, &snapshot); err != nil {
		e.t.Error("failed to unmarshal snapshot", err)
	}
	funcName := snapshot.Debugger.ProbeInSnapshot.Type + "." + snapshot.Debugger.ProbeInSnapshot.Method
	e.t.Logf("Received snapshot for function: %s", funcName)
	return len(p), nil
}

var (
	analyzedBinaries     []BinaryInfo
	waitForAttach        bool = true
	bufferPool                = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
	seenBinaries              = make(map[string]struct{})
	g_configsAccumulator *ConfigAccumulator
	g_RepoInfo           *RepoInfo
	g_ConfigManager      *diconfig.ReaderConfigManager
	g_cmd                *exec.Cmd
	DEBUG                bool = true
	TRACE                bool = false
	NUMBER_OF_PROBES     int  = 10

	explorationTestConfigTemplateText = `
    {{- range $index, $target := .}}
    {{- if $index}},{{end}}
    "{{$target.ProbeId}}": {
        "id": "{{$target.ProbeId}}",
        "version": 0,
        "type": "LOG_PROBE",
        "language": "go",
        "where": {
            "typeName": "{{$target.PackageName}}",
            "methodName": "{{$target.FunctionName}}"
        },
        "tags": [],
        "template": "Executed {{$target.PackageName}}.{{$target.FunctionName}}, it took {@duration}ms",
        "segments": [
            {
                "str": "Executed {{$target.PackageName}}.{{$target.FunctionName}}, it took "
            },
            {
                "dsl": "@duration",
                "json": {
                    "ref": "@duration"
                }
            },
            {
                "str": "ms"
            }
        ],
        "captureSnapshot": false,
        "capture": {
            "maxReferenceDepth": 10
        },
        "sampling": {
            "snapshotsPerSecond": 5000
        },
        "evaluateAt": "EXIT"
    }
    {{- end}}
`
)

func getProcessArgs(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return nil, err
	}
	args := strings.Split(string(data), "\x00")
	if len(args) > 0 && args[len(args)-1] == "" {
		args = args[:len(args)-1]
	}
	return args, nil
}

func getProcessCwd(pid int) (string, error) {
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "", err
	}
	return cwd, nil
}

func getProcessEnv(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil, err
	}
	env := strings.Split(string(data), "\x00")
	if len(env) > 0 && env[len(env)-1] == "" {
		env = env[:len(env)-1]
	}
	return env, nil
}

func extractDDService(env []string) (string, error) {
	for _, entry := range env {
		if strings.HasPrefix(entry, "DD_SERVICE=") {
			return strings.TrimPrefix(entry, "DD_SERVICE="), nil
		}
	}
	return "", fmt.Errorf("DD_SERVICE not found")
}

func hasDWARFInfo(binaryPath string) (bool, error) {
	f, err := elf.Open(binaryPath)
	if err != nil {
		return false, fmt.Errorf("failed to open binary: %w", err)
	}
	defer f.Close()

	debugSections := false
	for _, section := range f.Sections {
		if strings.HasPrefix(section.Name, ".debug_") {
			fmt.Printf("Found debug section: %s (size: %d)\n", section.Name, section.Size)
			debugSections = true
		}
	}

	dwarfData, err := f.DWARF()
	if err != nil {
		return debugSections, fmt.Errorf("DWARF read error: %w", err)
	}

	reader := dwarfData.Reader()
	entry, err := reader.Next()
	if err != nil {
		return debugSections, fmt.Errorf("DWARF entry read error: %w", err)
	}
	if entry != nil {
		fmt.Printf("Found DWARF entry of type: %v\n", entry.Tag)
		return true, nil
	}
	return false, nil
}

func listAllFunctions(filePath string) ([]FunctionInfo, error) {
	var functions []FunctionInfo
	var errors []string

	ef, err := elf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer ef.Close()

	dwarfData, err := ef.DWARF()
	if err != nil {
		return nil, fmt.Errorf("failed to load DWARF data: %v", err)
	}

	reader := dwarfData.Reader()
	for {
		entry, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("error reading DWARF entry: %v", err)
		}
		if entry == nil {
			break
		}
		if entry.Tag == dwarf.TagSubprogram {
			funcName, ok := entry.Val(dwarf.AttrName).(string)
			if !ok || funcName == "" {
				continue
			}
			info := extractPackageAndFunction(funcName)
			if info.FunctionName == "" {
				errors = append(errors, fmt.Sprintf("could not extract function name from %q", funcName))
				continue
			}
			functions = append(functions, info)
		}
	}

	if len(functions) == 0 {
		if len(errors) > 0 {
			return nil, fmt.Errorf("failed to extract any functions. Errors: %s", strings.Join(errors, "; "))
		}
		return nil, fmt.Errorf("no functions found in the binary")
	}
	return functions, nil
}

func extractPackageAndFunction(fullName string) FunctionInfo {
	if fullName == "" {
		return FunctionInfo{}
	}
	parenIndex := strings.Index(fullName, "(")
	lastDot := -1
	if parenIndex != -1 {
		lastDot = strings.LastIndex(fullName[:parenIndex], ".")
	} else {
		lastDot = strings.LastIndex(fullName, ".")
	}
	if lastDot == -1 {
		return FunctionInfo{}
	}
	pkgPath := fullName[:lastDot]
	funcPart := fullName[lastDot+1:]
	return NewFunctionInfo(pkgPath, funcPart, fullName)
}

func shouldProfileFunction(name string) bool {
	if strings.HasPrefix(name, "*ZN") ||
		strings.HasPrefix(name, "_") ||
		strings.Contains(name, "_sanitizer") ||
		strings.Contains(name, "runtime.") {
		return false
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return false
	}
	pkgPath := parts[0]
	if len(parts) > 2 {
		pkgPath = strings.Join(parts[:len(parts)-1], "/")
	}
	for repoPkg := range g_RepoInfo.Packages {
		if strings.Contains(pkgPath, repoPkg) {
			return true
		}
	}
	return false
}

func filterFunctions(funcs []FunctionInfo) []FunctionInfo {
	var validFuncs []FunctionInfo
	for _, f := range funcs {
		fullName := fmt.Sprintf("%s.%s", f.PackageName, f.FunctionName)
		if shouldProfileFunction(fullName) {
			validFuncs = append(validFuncs, f)
		}
	}
	if len(validFuncs) == 0 {
		return nil
	}
	sort.Slice(validFuncs, func(i, j int) bool {
		fullNameI := fmt.Sprintf("%s.%s", validFuncs[i].PackageName, validFuncs[i].FunctionName)
		fullNameJ := fmt.Sprintf("%s.%s", validFuncs[j].PackageName, validFuncs[j].FunctionName)
		return fullNameI < fullNameJ
	})
	if len(validFuncs) <= NUMBER_OF_PROBES {
		return validFuncs
	}
	return validFuncs[:NUMBER_OF_PROBES]
}

func ExtractFunctions(binaryPath string) ([]FunctionInfo, error) {
	file, err := elf.Open(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open binary: %v", err)
	}
	defer file.Close()

	dwarfData, err := file.DWARF()
	if err != nil {
		return nil, fmt.Errorf("failed to load DWARF data: %v", err)
	}

	var functions []FunctionInfo
	reader := dwarfData.Reader()
	for {
		entry, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("error reading DWARF: %v", err)
		}
		if entry == nil {
			break
		}
		if entry.Tag == dwarf.TagSubprogram {
			funcName, _ := entry.Val(dwarf.AttrName).(string)
			var packageName string
			if compDir, ok := entry.Val(dwarf.AttrCompDir).(string); ok {
				packageName = compDir
			}
			if funcName != "" {
				functions = append(functions, FunctionInfo{
					PackageName:  packageName,
					FunctionName: funcName,
				})
			}
		}
	}
	return functions, nil
}

func hasDWARF(binaryPath string) (bool, error) {
	file, err := elf.Open(binaryPath)
	if err != nil {
		return false, fmt.Errorf("failed to open binary: %v", err)
	}
	defer file.Close()

	_, err = file.DWARF()
	if err != nil {
		if err.Error() == "no DWARF data" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check DWARF data: %v", err)
	}
	return true, nil
}

func fingerprintGoBinary(binaryPath string) (string, error) {
	f, err := elf.Open(binaryPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sections := make([]*elf.Section, len(f.Sections))
	copy(sections, f.Sections)
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].Name < sections[j].Name
	})

	hash := sha256.New()
	for _, sec := range sections {
		if sec.Type == elf.SHT_NOBITS || sec.Name == ".note.go.buildid" {
			continue
		}
		if _, err := io.WriteString(hash, sec.Name); err != nil {
			return "", err
		}
		data, err := sec.Data()
		if err != nil {
			return "", err
		}
		if _, err := hash.Write(data); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func isAlreadyProcessed(binaryPath string) (bool, error) {
	fingerprint, err := fingerprintGoBinary(binaryPath)
	if err != nil {
		return false, err
	}
	if _, exists := seenBinaries[fingerprint]; exists {
		return true, nil
	}
	seenBinaries[fingerprint] = struct{}{}
	return false, nil
}

func getBinaryPath(pid int) string {
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = realPath
	}
	return path
}

func getParentPID(pid int) int {
	ppidStr, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(ppidStr))
	if len(fields) < 4 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[3])
	return ppid
}

func findAncestors(pid int, tree map[int]bool) {
	for pid > 1 {
		if tree[pid] {
			return
		}
		tree[pid] = true
		ppid := getParentPID(pid)
		if ppid <= 1 {
			return
		}
		pid = ppid
	}
}

func LogDebug(t *testing.T, format string, args ...any) {
	if DEBUG {
		t.Logf(format, args...)
	}
}

func NewProbeManager(t *testing.T) *ProbeManager {
	return &ProbeManager{t: t}
}

func (pm *ProbeManager) Install(pid int, function string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	v, _ := pm.installedProbes.LoadOrStore(pid, make(map[string]struct{}))
	probes := v.(map[string]struct{})
	probes[function] = struct{}{}
	pm.t.Logf("Installing probe: PID=%d Function=%s", pid, function)
	return nil
}

func (pm *ProbeManager) Remove(pid int, function string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if v, ok := pm.installedProbes.Load(pid); ok {
		probes := v.(map[string]struct{})
		delete(probes, function)
		pm.t.Logf("Removing probe: PID=%d Function=%s", pid, function)
	}
	return nil
}

func (pm *ProbeManager) CollectData(pid int, function string) (bool, error) {
	if v, ok := pm.dataReceived.Load(pid); ok {
		dataMap := v.(map[string]bool)
		return dataMap[function], nil
	}
	return false, nil
}

func NewProcessTracker(t *testing.T) *ProcessTracker {
	return &ProcessTracker{
		t:                t,
		processes:        make(map[int]*ProcessInfo),
		stopChan:         make(chan struct{}),
		analyzedBinaries: make(map[string]bool),
		analyzedPIDs:     make(map[int]bool),
		done:             make(chan struct{}),
	}
}

func (pt *ProcessTracker) markAnalyzed(pid int, path string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.analyzedPIDs[pid] = true
	pt.analyzedBinaries[path] = true
}

func (pt *ProcessTracker) addProcess(pid int, parentPID int) *ProcessInfo {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if proc, exists := pt.processes[pid]; exists {
		return proc
	}

	binaryPath := getBinaryPath(pid)
	proc := &ProcessInfo{
		PID:        pid,
		ParentPID:  parentPID,
		BinaryPath: binaryPath,
		State:      StateNew,
		StartTime:  time.Now(),
		Analyzed:   false,
	}
	pt.processes[pid] = proc
	if parent, exists := pt.processes[parentPID]; exists {
		parent.Children = append(parent.Children, proc)
	}
	pt.LogTrace("New process: PID=%d, Parent=%d, Binary=%s", pid, parentPID, binaryPath)
	return proc
}

func (pt *ProcessTracker) analyzeBinary(pid int, info *ProcessInfo) error {
	if info == nil {
		return fmt.Errorf("nil process info")
	}
	pt.mu.Lock()
	info.State = StateAnalyzing
	pt.mu.Unlock()

	if err := InspectBinary(pt.t, info.BinaryPath, pid); err != nil {
		pt.mu.Lock()
		info.State = StateNew
		pt.mu.Unlock()
		return fmt.Errorf("binary analysis failed: %v", err)
	}
	pt.mu.Lock()
	info.State = StateRunning
	pt.mu.Unlock()
	return nil
}

func (pt *ProcessTracker) scanProcessTree() error {
	if err := syscall.Kill(-g_cmd.Process.Pid, syscall.SIGSTOP); err != nil {
		if err != unix.ESRCH {
			pt.LogTrace("⚠️ Failed to stop PID %d: %v", -g_cmd.Process.Pid, err)
		}
		return nil
	}
	defer func() {
		if err := syscall.Kill(-g_cmd.Process.Pid, syscall.SIGCONT); err != nil {
			if err != unix.ESRCH {
				pt.LogTrace("⚠️ Failed to resume PID %d: %v", -g_cmd.Process.Pid, err)
			}
		}
	}()
	allPids := make(map[int]bool)
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, entry := range entries {
			if pid, err := strconv.Atoi(entry.Name()); err == nil {
				allPids[pid] = true
			}
		}
	}
	ourProcessTree := make(map[int]bool)
	ourPid := os.Getpid()
	findAncestors(ourPid, ourProcessTree)

	var toAnalyze []struct {
		pid  int
		path string
		ppid int
	}
	for pid := range allPids {
		pt.mu.RLock()
		if pt.analyzedPIDs[pid] {
			pt.mu.RUnlock()
			continue
		}
		pt.mu.RUnlock()
		if ourProcessTree[pid] {
			continue
		}
		binaryPath := getBinaryPath(pid)
		if binaryPath == "" {
			continue
		}
		ppid := getParentPID(pid)
		if ourProcessTree[ppid] {
			continue
		}
		shouldAnalyze := false
		if strings.HasSuffix(binaryPath, ".test") {
			shouldAnalyze = true
			pt.LogTrace("Found test binary: %s (PID=%d)", binaryPath, pid)
		} else if strings.Contains(binaryPath, "/go-build") && strings.Contains(binaryPath, "/exe/") {
			shouldAnalyze = true
			pt.LogTrace("Found build binary: %s (PID=%d)", binaryPath, pid)
		} else {
			parentPath := getBinaryPath(ppid)
			if strings.HasSuffix(parentPath, ".test") {
				shouldAnalyze = true
				pt.LogTrace("Found child of test: %s (PID=%d, Parent=%d)", binaryPath, pid, ppid)
			}
		}
		if shouldAnalyze {
			if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
				toAnalyze = append(toAnalyze, struct {
					pid  int
					path string
					ppid int
				}{pid, binaryPath, ppid})
				if pt.processes[pid] == nil {
					pt.addProcess(pid, ppid)
				}
			}
		}
	}
	if len(toAnalyze) > 0 {
		pt.LogTrace("🔍 Found %d processes to analyze:", len(toAnalyze))
		for _, p := range toAnalyze {
			pt.LogTrace("  PID=%d PPID=%d Path=%s", p.pid, p.ppid, p.path)
		}
	}
	var activePids []int
	for _, p := range toAnalyze {
		activePids = append(activePids, p.pid)
	}
	batchSize := 2
	for i := 0; i < len(toAnalyze); i += batchSize {
		end := i + batchSize
		if end > len(toAnalyze) {
			end = len(toAnalyze)
		}
		var wg sync.WaitGroup
		for _, p := range toAnalyze[i:end] {
			wg.Add(1)
			go func(pid int, path string) {
				defer wg.Done()
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
					return
				}
				pt.LogTrace("Stopping process for analysis: PID=%d Path=%s", pid, path)
				pt.mu.RLock()
				proc := pt.processes[pid]
				pt.mu.RUnlock()
				if proc == nil {
					return
				}
				if err := pt.analyzeBinary(pid, proc); err != nil {
					pt.LogTrace("Analysis failed: %v", err)
				} else {
					proc.Analyzed = true
					pt.markAnalyzed(pid, path)
				}
			}(p.pid, p.path)
		}
		wg.Wait()
		time.Sleep(10 * time.Microsecond)
	}
	return nil
}

func (pt *ProcessTracker) logProcessTree() {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	pt.t.Log("\n🌳 Process Tree:")
	var printNode func(proc *ProcessInfo, prefix string)
	printNode = func(proc *ProcessInfo, prefix string) {
		state := "➡️"
		switch proc.State {
		case StateAnalyzing:
			state = "🔍"
		case StateRunning:
			state = "▶️"
		case StateExited:
			state = "⏹️"
		}
		analyzed := ""
		if proc.Analyzed {
			analyzed = "✓"
		}
		pt.LogTrace("%s%s [PID=%d] %s%s (Parent=%d)",
			prefix, state, proc.PID, filepath.Base(proc.BinaryPath), analyzed, proc.ParentPID)
		for _, child := range proc.Children {
			printNode(child, prefix+"  ")
		}
	}
	if main, exists := pt.processes[pt.mainPID]; exists {
		printNode(main, "")
	}
}

func (pt *ProcessTracker) StartTracking(command string, args []string, dir string) error {
	cmd := exec.Command(command, args...)
	g_cmd = cmd
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"PWD="+dir,
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		"DD_SERVICE=go-di-exploration-test-service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	pt.mainPID = cmd.Process.Pid
	pt.addProcess(pt.mainPID, os.Getpid())

	go func() {
		initialTicker := time.NewTicker(1 * time.Millisecond)
		defer initialTicker.Stop()
		logTicker := time.NewTicker(10 * time.Second)
		defer logTicker.Stop()
		for {
			select {
			case <-pt.stopChan:
				return
			case <-initialTicker.C:
				if err := pt.scanProcessTree(); err != nil {
					pt.LogTrace("⚠️ Error scanning: %v", err)
				}
			case <-logTicker.C:
			}
		}
	}()

	err := cmd.Wait()
	close(pt.stopChan)
	pt.LogTrace("Analyzed %d binaries.", len(analyzedBinaries))
	for _, binary := range analyzedBinaries {
		pt.LogTrace("Analyzed %s (debug info: %v)", binary.path, binary.hasDebug)
	}
	return err
}

func (pt *ProcessTracker) Cleanup() {
	// Cleanup logic if needed.
}

func (pt *ProcessTracker) LogTrace(format string, args ...any) {
	if TRACE {
		pt.t.Logf(format, args...)
	}
}

func NewConfigAccumulator() (*ConfigAccumulator, error) {
	tmpl, err := template.New("config_template").Parse(explorationTestConfigTemplateText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	return &ConfigAccumulator{
		configs: make(map[string]map[string]rcConfig),
		tmpl:    tmpl,
	}, nil
}

func (ca *ConfigAccumulator) AddTargets(targets []FunctionInfo, serviceName string) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	buf.WriteString("{")
	if err := ca.tmpl.Execute(buf, targets); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	buf.WriteString("}")

	var newConfigs map[string]rcConfig
	if err := json.NewDecoder(buf).Decode(&newConfigs); err != nil {
		return fmt.Errorf("failed to decode generated configs: %w", err)
	}
	if ca.configs[serviceName] == nil {
		ca.configs[serviceName] = make(map[string]rcConfig)
	}
	for probeID, config := range newConfigs {
		ca.configs[serviceName][probeID] = config
	}
	return nil
}

func (ca *ConfigAccumulator) WriteConfigs() error {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	if err := json.NewEncoder(buf).Encode(ca.configs); err != nil {
		return fmt.Errorf("failed to marshal configs: %w", err)
	}
	return g_ConfigManager.ConfigWriter.WriteSync(buf.Bytes())
}

func InspectBinary(t *testing.T, binaryPath string, pid int) error {
	allFuncs, err := listAllFunctions(binaryPath)
	if err != nil {
		analyzedBinaries = append(analyzedBinaries, BinaryInfo{
			path:     binaryPath,
			hasDebug: false,
		})
		return nil
	}

	targets := filterFunctions(allFuncs)
	args, err := getProcessArgs(pid)
	if err != nil {
		return fmt.Errorf("failed to process args: %v", err)
	}
	cwd, err := getProcessCwd(pid)
	if err != nil {
		return fmt.Errorf("failed to get Cwd: %v", err)
	}
	env, err := getProcessEnv(pid)
	if err != nil {
		return fmt.Errorf("failed to get Env: %v", err)
	}
	serviceName, err := extractDDService(env)
	if err != nil {
		return fmt.Errorf("failed to get Env: %v, binaryPath: %s", err, binaryPath)
	}

	LogDebug(t, "\n=======================================")
	LogDebug(t, "🔍 SERVICE NAME: %s", serviceName)
	LogDebug(t, "🔍 ANALYZING BINARY: %s", binaryPath)
	LogDebug(t, "🔍 ENV: %v", env)
	LogDebug(t, "🔍 ARGS: %v", args)
	LogDebug(t, "🔍 CWD: %s", cwd)
	LogDebug(t, "🔍 Elected %d target functions:", len(targets))
	for _, f := range targets {
		LogDebug(t, "  → Package: %s, Function: %s, FullName: %s", f.PackageName, f.FunctionName, f.FullName)
	}
	LogDebug(t, "=======================================")

	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("(1) binary inspection failed: %v", err)
	}

	analyzedBinaries = append(analyzedBinaries, BinaryInfo{
		path:     binaryPath,
		hasDebug: len(targets) > 0,
	})
	LogDebug(t, "✅ Analysis complete for: %s", binaryPath)
	LogDebug(t, "=======================================\n")

	g_ConfigManager.ProcTracker.HandleProcessStartSync(uint32(pid))
	t.Logf("About to request instrumentations for binary: %s, pid: %d.", binaryPath, pid)

	if err := g_configsAccumulator.AddTargets(targets, serviceName); err != nil {
		t.Logf("Error adding target: %v, binaryPath: %s", err, binaryPath)
		return fmt.Errorf("add targets failed: %v, binary: %s", err, binaryPath)
	}
	if err = g_configsAccumulator.WriteConfigs(); err != nil {
		t.Logf("Error writing configs: %v, binaryPath: %s", err, binaryPath)
		return fmt.Errorf("error adding configs: %v, binary: %s", err, binaryPath)
	}
	time.Sleep(2 * time.Second)
	t.Logf("Requested to instrument %d functions for binary: %s, pid: %d.", len(targets), binaryPath, pid)
	for _, f := range targets {
		t.Logf("      -> requested instrumentation for %v", f)
	}
	if waitForAttach && os.Getenv("DEBUG") == "true" {
		pid := os.Getpid()
		t.Logf("Waiting to attach for PID: %d", pid)
		time.Sleep(30 * time.Second)
		waitForAttach = false
	}
	return nil
}

// TestExplorationGoDI is the entrypoint of the integration test of Go DI. The idea is to
// test Go DI systematically and in exploratory manner. In high level, here are the steps this test takes:
// 1. Clones protobuf and applies patches.
// 2. Figuring out the 1st party packages involved with the cloned project (to avoid 3rd party/std libs)
// 3. Compiles the test
// 4. Runs the test in a supervised environment, spawning processes as a group.
// 5. Periodically pauses and resumes the process group to analyze each binary unique.
// 6. Invoke Go DI to put probes in top X functions defined by `NUMBER_OF_RROBES` const.
//
//	The goal is to exercise as many code paths as possible of the Go DI system.
func TestExplorationGoDI(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock(), "Failed to remove memlock limit")
	if features.HaveMapType(ebpf.RingBuf) != nil {
		t.Skip("Ringbuffers not supported on this kernel")
	}

	eventOutputWriter := &explorationEventOutputTestWriter{t: t}
	opts := &dynamicinstrumentation.DIOptions{
		RateLimitPerProbePerSecond: 0.0,
		ReaderWriterOptions: dynamicinstrumentation.ReaderWriterOptions{
			CustomReaderWriters: true,
			SnapshotWriter:      eventOutputWriter,
			DiagnosticWriter:    os.Stderr,
		},
	}

	var (
		GoDI *dynamicinstrumentation.GoDI
		err  error
	)
	GoDI, err = dynamicinstrumentation.RunDynamicInstrumentation(opts)
	require.NoError(t, err)
	t.Cleanup(GoDI.Close)

	cm, ok := GoDI.ConfigManager.(*diconfig.ReaderConfigManager)
	if !ok {
		t.Fatal("Config manager is of wrong type")
	}
	g_ConfigManager = cm

	g_configsAccumulator, err = NewConfigAccumulator()
	if err != nil {
		t.Fatal("Failed to create ConfigAccumulator")
	}

	tempDir := initializeTempDir(t, "/tmp/protobuf-integration-1060272402")
	modulePath := filepath.Join(tempDir, "src", "google.golang.org", "protobuf")

	t.Log("Setting up test environment...")
	g_RepoInfo = cloneProtobufRepo(t, modulePath, "30f628eeb303f2c29be7a381bf78aa3e3aabd317")
	copyPatches(t, "exploration_tests/patches/protobuf", modulePath)

	t.Log("Starting process tracking...")
	tracker := NewProcessTracker(t)
	err = tracker.StartTracking("./test.bash", nil, modulePath)
	require.NoError(t, err)
}

func initializeTempDir(t *testing.T, predefinedTempDir string) string {
	if predefinedTempDir != "" {
		return predefinedTempDir
	}
	tempDir, err := os.MkdirTemp("", "protobuf-integration-")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(tempDir, 0755))
	t.Log("tempDir:", tempDir)
	return tempDir
}

func ScanRepoPackages(repoPath string) (*RepoInfo, error) {
	info := &RepoInfo{
		Packages: make(map[string]bool),
		RepoPath: repoPath,
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		if hash, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output(); err == nil {
			info.CommitHash = strings.TrimSpace(string(hash))
		}
	}
	err := filepath.Walk(repoPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if f.IsDir() {
			dirname := filepath.Base(path)
			if dirname == ".git" ||
				dirname == ".cache" ||
				dirname == "vendor" ||
				dirname == "testdata" ||
				strings.HasPrefix(dirname, ".") ||
				strings.HasPrefix(dirname, "tmp") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, ".pb.go") {
			return nil
		}
		relPath, err := filepath.Rel(repoPath, path)
		if err != nil || strings.Contains(relPath, "..") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		scanner := bufio.NewScanner(bytes.NewReader(content))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "package ") {
				pkgDir := filepath.Dir(relPath)
				if pkgDir != "." {
					info.Packages[pkgDir] = true
				}
				break
			}
		}
		return nil
	})
	if len(info.Packages) == 0 {
		return nil, fmt.Errorf("no packages found in repository at %s", repoPath)
	}
	return info, err
}

func cloneProtobufRepo(t *testing.T, modulePath string, commitHash string) *RepoInfo {
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		cmd := exec.Command("git", "clone", "https://github.com/protocolbuffers/protobuf-go", modulePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		require.NoError(t, cmd.Run(), "Failed to clone repository")
	}
	if commitHash != "" {
		cmd := exec.Command("git", "checkout", commitHash)
		cmd.Dir = modulePath
		require.NoError(t, cmd.Run(), "Failed to checkout commit hash")
	}
	info, err := ScanRepoPackages(modulePath)
	require.NoError(t, err, "Failed to scan repo packages")

	var pkgs []string
	for pkg := range info.Packages {
		if strings.Contains(pkg, "/tmp") {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	t.Logf("📦 Found %d packages in protobuf repo:", len(pkgs))

	groups := make(map[string][]string)
	for _, pkg := range pkgs {
		parts := strings.SplitN(pkg, "/", 2)
		topLevel := parts[0]
		groups[topLevel] = append(groups[topLevel], pkg)
	}

	var topLevels []string
	for k := range groups {
		topLevels = append(topLevels, k)
	}
	sort.Strings(topLevels)
	for _, topLevel := range topLevels {
		t.Logf("  %s/", topLevel)
		for _, pkg := range groups[topLevel] {
			t.Logf("    → %s", pkg)
		}
	}
	return info
}

func copyPatches(t *testing.T, src, dst string) {
	require.NoError(t, copyDir(src, dst), "Failed to copy patches")
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err = copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err = copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(srcFile, dstFile string) error {
	src, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer src.Close()
	if err = os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
		return err
	}
	dst, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}
