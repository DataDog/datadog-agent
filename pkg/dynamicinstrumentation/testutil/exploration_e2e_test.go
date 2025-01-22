// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"bufio"
	"bytes"
	"debug/dwarf"
	"debug/elf"
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

func NewProbeManager(t *testing.T) *ProbeManager {
	return &ProbeManager{
		t: t,
	}
}

func (pm *ProbeManager) Install(pid int, function string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Get or create the map of installed probes for this PID
	v, _ := pm.installedProbes.LoadOrStore(pid, make(map[string]struct{}))
	probes := v.(map[string]struct{})

	// Install the probe
	probes[function] = struct{}{}
	pm.t.Logf("🔧 Installing probe: PID=%d Function=%s", pid, function)

	// Your actual probe installation logic here using GoDI
	// Example:
	// err := pm.godi.InstallProbe(pid, function)
	return nil
}

func (pm *ProbeManager) Remove(pid int, function string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if v, ok := pm.installedProbes.Load(pid); ok {
		probes := v.(map[string]struct{})
		delete(probes, function)
		pm.t.Logf("🔧 Removing probe: PID=%d Function=%s", pid, function)

		// Your actual probe removal logic here
	}
	return nil
}

func (pm *ProbeManager) CollectData(pid int, function string) (bool, error) {
	// Check if we've received data for this probe
	// This is where you'd check your actual data collection mechanism

	// For testing, let's simulate data collection
	// In reality, you'd check if your probe has published any data
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

func getProcessArgs(pid int) ([]string, error) {
	// Construct the path to the /proc/<pid>/cmdline file
	procFile := fmt.Sprintf("/proc/%d/cmdline", pid)

	// Read the file content
	data, err := os.ReadFile(procFile)
	if err != nil {
		return nil, err
	}

	// The arguments are null-byte separated, split them
	args := strings.Split(string(data), "\x00")
	// Remove any trailing empty string caused by the trailing null byte
	if len(args) > 0 && args[len(args)-1] == "" {
		args = args[:len(args)-1]
	}
	return args, nil
}

func getProcessCwd(pid int) (string, error) {
	// Construct the path to the /proc/<pid>/cwd symlink
	procFile := fmt.Sprintf("/proc/%d/cwd", pid)

	// Read the symlink to find the current working directory
	cwd, err := os.Readlink(procFile)
	if err != nil {
		return "", err
	}
	return cwd, nil
}

func getProcessEnv(pid int) ([]string, error) {
	// Construct the path to the /proc/<pid>/environ file
	procFile := fmt.Sprintf("/proc/%d/environ", pid)

	// Open and read the file
	data, err := os.ReadFile(procFile)
	if err != nil {
		return nil, err
	}

	// The environment variables are null-byte separated, split them
	env := strings.Split(string(data), "\x00")
	// Remove any trailing empty string caused by the trailing null byte
	if len(env) > 0 && env[len(env)-1] == "" {
		env = env[:len(env)-1]
	}
	return env, nil
}

func hasDWARFInfo(binaryPath string) (bool, error) {
	f, err := elf.Open(binaryPath)
	if err != nil {
		return false, fmt.Errorf("failed to open binary: %w", err)
	}
	defer f.Close()

	// Try both approaches: section lookup and DWARF data reading
	debugSections := false
	for _, section := range f.Sections {
		if strings.HasPrefix(section.Name, ".debug_") {
			fmt.Printf("Found debug section: %s (size: %d)\n", section.Name, section.Size)
			debugSections = true
		}
	}

	// Try to actually read DWARF data
	dwarfData, err := f.DWARF()
	if err != nil {
		return debugSections, fmt.Errorf("DWARF read error: %w", err)
	}

	// Verify we can read some DWARF data
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

func extractPackageAndFunction(fullName string) FunctionInfo {
	// Handle empty input
	if fullName == "" {
		return FunctionInfo{}
	}

	// First, find the last index of "." before any parentheses
	parenIndex := strings.Index(fullName, "(")
	lastDot := -1
	if parenIndex != -1 {
		// If we have parentheses, look for the last dot before them
		lastDot = strings.LastIndex(fullName[:parenIndex], ".")
	} else {
		// If no parentheses, just find the last dot
		lastDot = strings.LastIndex(fullName, ".")
	}

	if lastDot == -1 {
		return FunctionInfo{}
	}

	// Split into package and function parts
	pkgPath := fullName[:lastDot]
	funcPart := fullName[lastDot+1:]

	return NewFunctionInfo(pkgPath, funcPart, fullName)
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

// func isStandardPackage(pkg string) bool {
//  // List of common standard library packages that might be nested
//  stdPkgs := map[string]bool{
//      "encoding/json":   true,
//      "compress/flate":  true,
//      "compress/gzip":   true,
//      "encoding/base64": true,
//      // Add more as needed
//  }
//  return stdPkgs[pkg]
// }

// func listAllFunctions(filePath string) ([]FunctionInfo, error) {
//  var functions []FunctionInfo

//  // Open the ELF file
//  ef, err := elf.Open(filePath)
//  if err != nil {
//      return nil, fmt.Errorf("failed to open file: %v", err)
//  }
//  defer ef.Close()

//  // Retrieve symbols from the ELF file
//  symbols, err := ef.Symbols()
//  if err != nil {
//      return nil, fmt.Errorf("failed to read symbols: %v", err)
//  }

//  // Iterate over symbols and filter function symbols
//  for _, sym := range symbols {
//      if elf.ST_TYPE(sym.Info) == elf.STT_FUNC {
//          // Extract function name
//          functionName := sym.Name

//          // Extract package name from section index (if applicable)
//          // DWARF data or additional analysis can refine this
//          packageName := ""

//          // Add to result
//          functions = append(functions, FunctionInfo{
//              PackageName:  packageName,
//              FunctionName: functionName,
//          })
//      }
//  }
//  return functions, nil
// }

func shouldProfileFunction(name string) bool {
	// First, immediately reject known system/internal functions
	if strings.HasPrefix(name, "*ZN") || // Sanitizer/LLVM functions
		strings.HasPrefix(name, "_") || // Internal functions
		strings.Contains(name, "_sanitizer") ||
		strings.Contains(name, "runtime.") {
		return false
	}

	// Extract package from function name
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return false
	}

	pkgPath := parts[0]
	if len(parts) > 2 {
		pkgPath = strings.Join(parts[:len(parts)-1], "/")
	}

	// Check if it's in our repository packages
	for repoPkg := range g_RepoInfo.Packages {
		if strings.Contains(pkgPath, repoPkg) {
			return true
		}
	}

	return false
}

// func shouldProfileFunction(name string) bool {
//  // Skip standard library packages
//  stdlibPrefixes := []string{
//      "bufio.",
//      "bytes.",
//      "context.",
//      "crypto.",
//      "compress/",
//      "database/",
//      "debug/",
//      "encoding/",
//      "errors.",
//      "flag.",
//      "fmt.",
//      "io.",
//      "log.",
//      "math.",
//      "net.",
//      "os.",
//      "path.",
//      "reflect.",
//      "regexp.",
//      "runtime.",
//      "sort.",
//      "strconv.",
//      "strings.",
//      "sync.",
//      "syscall.",
//      "time.",
//      "unicode.",
//  }

//  // Definitely skip these system internals
//  skipPrefixes := []string{
//      "runtime.",
//      "runtime/race",
//      "*ZN",      // LLVM/Clang internals
//      "type..",   // Go type metadata
//      "gc.",      // Garbage collector
//      "gosb.",    // Go sandbox
//      "_rt.",     // Runtime helpers
//      "reflect.", // Reflection internals
//  }

//  skipContains := []string{
//      "_sanitizer",
//      "_tsan",
//      ".constprop.", // Compiler generated constants
//      ".isra.",      // LLVM optimized functions
//      ".part.",      // Partial functions from compiler
//      "__gcc_",      // GCC internals
//      "_cgo_",       // CGO generated code
//      "goexit",      // Go runtime exit handlers
//      "gcproc",      // GC procedures
//      ".loc.",       // Location metadata
//      "runtime·",    // Runtime internals (different dot)
//  }

//  // Quick reject for standard library and system functions
//  for _, prefix := range append(stdlibPrefixes, skipPrefixes...) {
//      if strings.HasPrefix(name, prefix) {
//          return false
//      }
//  }

//  for _, substr := range skipContains {
//      if strings.Contains(name, substr) {
//          return false
//      }
//  }

//  // High priority user functions - definitely profile these
//  priorityPrefixes := []string{
//      "main.",
//      "cmd.",
//      "github.com/",
//      "golang.org/x/",
//      "google.golang.org/",
//      "k8s.io/",
//  }

//  for _, prefix := range priorityPrefixes {
//      if strings.HasPrefix(name, prefix) {
//          return true
//      }
//  }

//  // Function looks like a normal Go function (CapitalizedName)
//  if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
//      return true
//  }

//  // If it contains a dot and doesn't look like a compiler-generated name
//  if strings.Contains(name, ".") &&
//      !strings.Contains(name, "$") &&
//      !strings.Contains(name, "__") {
//      return true
//  }

//  // If we get here, it's probably a system function
//  return false
// }

var NUMBER_OF_PROBES int = 100

func filterFunctions(funcs []FunctionInfo) []FunctionInfo {
	var validFuncs []FunctionInfo

	// First pass: collect only functions from our packages
	for _, f := range funcs {
		// Combine package and function name for filtering
		fullName := fmt.Sprintf("%s.%s", f.PackageName, f.FunctionName)
		if shouldProfileFunction(fullName) {
			validFuncs = append(validFuncs, f)
		}
	}

	// If we have no valid functions, return empty list
	if len(validFuncs) == 0 {
		return nil
	}

	// Sort valid functions for consistent ordering
	sort.Slice(validFuncs, func(i, j int) bool {
		// Sort alphabetically by full name (package + function)
		fullNameI := fmt.Sprintf("%s.%s", validFuncs[i].PackageName, validFuncs[i].FunctionName)
		fullNameJ := fmt.Sprintf("%s.%s", validFuncs[j].PackageName, validFuncs[j].FunctionName)
		return fullNameI < fullNameJ
	})

	// Return all if we have 10 or fewer
	if len(validFuncs) <= NUMBER_OF_PROBES {
		return validFuncs
	}

	// Only take first 10 if we have more
	return validFuncs[:NUMBER_OF_PROBES]
}

// func filterFunctions(funcs []string) []string {
//  var validFuncs []string

//  // First pass: collect only functions from our packages
//  for _, f := range funcs {
//      if shouldProfileFunction(f) {
//          validFuncs = append(validFuncs, f)
//      }
//  }

//  // If we have no valid functions, return empty list
//  if len(validFuncs) == 0 {
//      return nil
//  }

//  // Sort for consistent ordering
//  sort.Strings(validFuncs)

//  // Return all if we have 10 or fewer
//  if len(validFuncs) <= NUMBER_OF_PROBES {
//      return validFuncs
//  }

//  // Only take first 10 if we have more
//  return validFuncs[:NUMBER_OF_PROBES]
// }

func ExtractFunctions(binaryPath string) ([]FunctionInfo, error) {
	// Open the binary
	file, err := elf.Open(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open binary: %v", err)
	}
	defer file.Close()

	// Get DWARF data
	dwarfData, err := file.DWARF()
	if err != nil {
		return nil, fmt.Errorf("failed to load DWARF data: %v", err)
	}

	// Prepare result
	var functions []FunctionInfo

	// Iterate over DWARF entries
	reader := dwarfData.Reader()
	for {
		entry, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("error reading DWARF: %v", err)
		}
		if entry == nil {
			break // End of entries
		}

		// Check for subprogram (function) entries
		if entry.Tag == dwarf.TagSubprogram {
			// Extract function name
			funcName, _ := entry.Val(dwarf.AttrName).(string)

			// Extract package/module name (if available)
			var packageName string
			if compDir, ok := entry.Val(dwarf.AttrCompDir).(string); ok {
				packageName = compDir
			}

			// Add to the result
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

// hasDWARF checks if the given binary contains DWARF debug information.
func hasDWARF(binaryPath string) (bool, error) {
	// Open the binary file
	file, err := elf.Open(binaryPath)
	if err != nil {
		return false, fmt.Errorf("failed to open binary: %v", err)
	}
	defer file.Close()

	// Check if DWARF data exists
	_, err = file.DWARF()
	if err != nil {
		// Check if the error indicates missing DWARF information
		if err.Error() == "no DWARF data" {
			return false, nil
		}
		// Otherwise, propagate the error
		return false, fmt.Errorf("failed to check DWARF data: %v", err)
	}

	// DWARF data exists
	return true, nil
}

var analyzedBinaries []BinaryInfo
var waitForAttach bool = true

func InspectBinary(t *testing.T, binaryPath string, pid int) error {
	// // check that we can analyse the binary without targeting a specific function
	// err := diconfig.AnalyzeBinary(&ditypes.ProcessInfo{BinaryPath: binaryPath})
	// if err != nil {
	//  // log.Fatalln("Failed to analyze", binaryPath, "--", err)
	//  return nil
	// }

	// targets, err := ExtractFunctions(binaryPath)
	// if err != nil {
	//  // log.Fatalf("Error extracting functions: %v", err)
	//  return nil
	// }

	// hasDwarf, err := hasDWARF(binaryPath)
	// if err != nil || !hasDwarf {
	//  // log.Fatalf("Error checking for DWARF info: %v", err)
	//  return nil
	// }

	allFuncs, err := listAllFunctions(binaryPath)
	if err != nil {
		analyzedBinaries = append(analyzedBinaries, BinaryInfo{
			path:     binaryPath,
			hasDebug: false,
		})

		return nil
	}

	// targets := filterFunctions(allFuncs)
	targets := allFuncs

	// Get process arguments
	args, err := getProcessArgs(pid)
	if err != nil {
		return fmt.Errorf("Failed to process args: %v", err)
	}

	// Get process current working directory
	cwd, err := getProcessCwd(pid)
	if err != nil {
		return fmt.Errorf("Failed to get Cwd: %v", err)
	}

	// // Get process environment variables
	// env, err := getProcessEnv(pid)
	// if err != nil {
	//  return fmt.Errorf("Failed to get Env: %v", err)
	// }

	LogDebug(t, "\n=======================================")
	LogDebug(t, "🔍 ANALYZING BINARY: %s", binaryPath)
	LogDebug(t, "🔍 ARGS: %v", args)
	LogDebug(t, "🔍 CWD: %s", cwd)
	LogDebug(t, "🔍 Elected %d target functions:", len(targets))
	for _, f := range targets {
		LogDebug(t, "  → Package: %s, Function: %s, FullName: %s", f.PackageName, f.FunctionName, f.FullName)
	}

	// hasDWARF, dwarfErr := hasDWARFInfo(binaryPath)
	// if dwarfErr != nil {
	//  log.Printf("Error checking DWARF info: %v", dwarfErr)
	// } else {
	//  log.Printf("Binary has DWARF info: %v", hasDWARF)
	// }
	// LogDebug(t, "🔍 ENV: %v", env)
	LogDebug(t, "=======================================")

	// Check if the binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("(1) binary inspection failed: %v", err)
	}

	analyzedBinaries = append(analyzedBinaries, BinaryInfo{
		path:     binaryPath,
		hasDebug: len(targets) > 0,
	})

	// i := 0
	// // Re-check binary existence
	// for {
	//  if _, err := os.Stat(binaryPath); err != nil {
	//      time.Sleep(10 * time.Hour)
	//      return fmt.Errorf("(2) binary inspection failed: %v", err)
	//  }

	//  // if strings.HasSuffix(binaryPath, "generate-protos") {
	//  //  break
	//  // }

	//  if strings.HasSuffix(binaryPath, "conformance.test") {
	//      time.Sleep(10 * time.Second)
	//      break
	//  }

	//  i++
	//  if i > 11 {
	//      break
	//  }

	//  //  time.Sleep(100 * time.Millisecond)
	// }

	LogDebug(t, "✅ Analysis complete for: %s", binaryPath)
	LogDebug(t, "=======================================\n")

	t.Logf("About to request instrumentations for binary: %s, pid: %d.", binaryPath, pid)

	cfgTemplate, err := template.New("config_template").Parse(explorationTestConfigTemplateText)
	require.NoError(t, err)

	b := []byte{}
	var buf *bytes.Buffer

	// if waitForAttach {
	//  pid := os.Getpid()
	//  t.Logf("(1) Waiting to attach for PID: %d", pid)
	//  time.Sleep(30 * time.Second)
	//  waitForAttach = false
	// }

	requesterdFuncs := 0
	for _, f := range targets {

		// if !strings.Contains(f.FullName, "blabla_blabla") {
		// 	continue
		// }

		if !strings.Contains(f.FullName, "FullName") {
			continue
		}

		// if f.FullName != "regexp.(*bitState).shouldVisit" {
		// 	continue
		// }

		// if f.FullName != "google.golang.org/protobuf/encoding/protodelim_test.(*notBufioReader).UnreadRune" {
		//  continue
		// }

		buf = bytes.NewBuffer(b)
		err = cfgTemplate.Execute(buf, f)
		if err != nil {
			continue
		}

		// LogDebug(t, "Requesting instrumentation for %v", f)
		t.Logf("Requesting instrumentation for %v", f)
		_, err := g_ConfigManager.ConfigWriter.Write(buf.Bytes())

		if err != nil {
			continue
		}

		requesterdFuncs++
	}

	if !waitForAttach {
		time.Sleep(100 * time.Second)
	}

	if requesterdFuncs > 0 {
		// if waitForAttach {
		//  pid := os.Getpid()
		//  t.Logf("(2) Waiting to attach for PID: %d", pid)
		//  time.Sleep(30 * time.Second)
		//  waitForAttach = false
		// }

		// Wait for probes to be instrumented
		time.Sleep(2 * time.Second)

		t.Logf("Requested to instrument %d functions for binary: %s, pid: %d.", requesterdFuncs, binaryPath, pid)
	}

	return nil
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

	// Add to parent's children if parent exists
	if parent, exists := pt.processes[parentPID]; exists {
		parent.Children = append(parent.Children, proc)
	}

	pt.LogTrace("👶 New process: PID=%d, Parent=%d, Binary=%s", pid, parentPID, binaryPath)
	return proc
}

func getBinaryPath(pid int) string {
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}

	// Resolve any symlinks
	realPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = realPath
	}

	return path
}

func (pt *ProcessTracker) analyzeBinary(pid int, info *ProcessInfo) error {
	if info == nil {
		return fmt.Errorf("nil process info")
	}

	pt.mu.Lock()
	info.State = StateAnalyzing
	pt.mu.Unlock()

	// pt.LogTrace("🔎 Analyzing binary PID=%d Path=%s", pid, info.BinaryPath)

	// Perform analysis
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

func (pt *ProcessTracker) scanProcessTree() error {
	if err := syscall.Kill(-g_cmd.Process.Pid, syscall.SIGSTOP); err != nil {
		if err != unix.ESRCH {
			pt.LogTrace("⚠️ Failed to stop PID %d: %v", -g_cmd.Process.Pid, err)
		}
		return nil
	}

	// pt.profiler.OnProcessesPaused()

	defer func() {
		if err := syscall.Kill(-g_cmd.Process.Pid, syscall.SIGCONT); err != nil {
			if err != unix.ESRCH {
				pt.LogTrace("⚠️ Failed to resume PID %d: %v", -g_cmd.Process.Pid, err)
			}
		} else {
			// pt.LogTrace("▶️ Resumed process: PID=%d", -g_cmd.Process.Pid)
		}

		// pt.profiler.OnProcessesResumed()

		// if err := unix.Kill(pid, unix.SIGCONT); err != nil {
		//  if err != unix.ESRCH {
		//      pt.LogTrace("⚠️ Failed to resume PID %d: %v", pid, err)
		//  }
		// } else {
		//  pt.LogTrace("▶️ Resumed process: PID=%d", pid)
		// }
	}()

	// Get all processes
	allPids := make(map[int]bool)
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, entry := range entries {
			if pid, err := strconv.Atoi(entry.Name()); err == nil {
				allPids[pid] = true
			}
		}
	}

	// Record our own process tree for exclusion
	ourProcessTree := make(map[int]bool)
	ourPid := os.Getpid()
	findAncestors(ourPid, ourProcessTree)

	var toAnalyze []struct {
		pid  int
		path string
		ppid int
	}

	// Check each PID
	for pid := range allPids {
		// Skip if already analyzed
		pt.mu.RLock()
		if pt.analyzedPIDs[pid] {
			pt.mu.RUnlock()
			continue
		}
		pt.mu.RUnlock()

		// Skip if in our process tree
		if ourProcessTree[pid] {
			continue
		}

		// Get process path
		binaryPath := getBinaryPath(pid)
		if binaryPath == "" {
			continue
		}

		// Get parent PID
		ppid := getParentPID(pid)

		// Skip if parent is in our tree
		if ourProcessTree[ppid] {
			continue
		}

		// Always analyze:
		// 1. Test binaries (.test)
		// 2. Go build executables in /tmp
		// 3. Children of test binaries
		shouldAnalyze := false

		if strings.HasSuffix(binaryPath, ".test") {
			shouldAnalyze = true
			pt.LogTrace("Found test binary: %s (PID=%d)", binaryPath, pid)
		} else if strings.Contains(binaryPath, "/go-build") && strings.Contains(binaryPath, "/exe/") {
			shouldAnalyze = true
			pt.LogTrace("Found build binary: %s (PID=%d)", binaryPath, pid)
		} else {
			// Check if parent is a test binary
			parentPath := getBinaryPath(ppid)
			if strings.HasSuffix(parentPath, ".test") {
				shouldAnalyze = true
				pt.LogTrace("Found child of test: %s (PID=%d, Parent=%d)", binaryPath, pid, ppid)
			}
		}

		if shouldAnalyze {
			// Verify process still exists
			if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
				toAnalyze = append(toAnalyze, struct {
					pid  int
					path string
					ppid int
				}{pid, binaryPath, ppid})

				// Add to process tree
				if pt.processes[pid] == nil {
					pt.addProcess(pid, ppid)
				}
			}
		}
	}

	if len(toAnalyze) > 0 {
		pt.LogTrace("\n🔍 Found %d processes to analyze:", len(toAnalyze))
		for _, p := range toAnalyze {
			pt.LogTrace("  PID=%d PPID=%d Path=%s", p.pid, p.ppid, p.path)
		}
	}

	var activePids []int
	for _, p := range toAnalyze {
		activePids = append(activePids, p.pid)
	}

	// if pt.profiler!= nil {
	//  pt.profiler.OnTick(activePids)
	// }

	// Process in small batches
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

				// Verify process still exists
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
					return
				}

				pt.LogTrace("🔐 Stopping process for analysis: PID=%d Path=%s", pid, path)

				// Get process info
				pt.mu.RLock()
				proc := pt.processes[pid]
				pt.mu.RUnlock()

				if proc == nil {
					return
				}

				// Stop process
				// if err := syscall.Kill(-g_cmd.Process.Pid, syscall.SIGSTOP); err != nil {
				//  if err != unix.ESRCH {
				//      pt.LogTrace("⚠️ Failed to stop PID %d: %v", pid, err)
				//  }
				//  return
				// }

				// if err := unix.Kill(pid, unix.SIGSTOP); err != nil {
				//  if err != unix.ESRCH {
				//      pt.LogTrace("⚠️ Failed to stop PID %d: %v", pid, err)
				//  }
				//  return
				// }

				// Ensure process gets resumed
				// defer func() {
				//  if err := syscall.Kill(-g_cmd.Process.Pid, syscall.SIGCONT); err != nil {
				//      if err != unix.ESRCH {
				//          pt.LogTrace("⚠️ Failed to resume PID %d: %v", pid, err)
				//      }
				//  } else {
				//      pt.LogTrace("▶️ Resumed process: PID=%d", pid)
				//  }

				//  // if err := unix.Kill(pid, unix.SIGCONT); err != nil {
				//  //  if err != unix.ESRCH {
				//  //      pt.LogTrace("⚠️ Failed to resume PID %d: %v", pid, err)
				//  //  }
				//  // } else {
				//  //  pt.LogTrace("▶️ Resumed process: PID=%d", pid)
				//  // }
				// }()

				// Wait a bit after stopping
				// time.Sleep(1 * time.Millisecond)

				// Analyze with timeout
				if err := pt.analyzeBinary(pid, proc); err != nil {
					pt.LogTrace("⚠️ Analysis failed: %v", err)
				} else {
					proc.Analyzed = true
					pt.markAnalyzed(pid, path)
					// pt.LogTrace("✅ Analysis complete: PID=%d", pid)
				}

				// go func() {
				//  if err := pt.analyzeBinary(pid, proc); err != nil {
				//      pt.LogTrace("⚠️ Analysis failed: %v", err)
				//      done <- false
				//      return
				//  }

				//  proc.Analyzed = true
				//  pt.markAnalyzed(pid, path)
				//  pt.LogTrace("✅ Analysis complete: PID=%d", pid)
				//  done <- true
				// }()
			}(p.pid, p.path)
		}
		wg.Wait()

		// Wait between batches
		time.Sleep(10 * time.Microsecond)
	}

	return nil
}

func (pt *ProcessTracker) Cleanup() {
}

// Helper to record process tree starting from a PID
func findAncestors(pid int, tree map[int]bool) {
	for pid > 1 {
		if tree[pid] {
			return // Already visited
		}
		tree[pid] = true

		// Get parent
		ppid := getParentPID(pid)
		if ppid <= 1 {
			return
		}
		pid = ppid
	}
}

var g_cmd *exec.Cmd

func (pt *ProcessTracker) StartTracking(command string, args []string, dir string) error {
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	// if err := pt.profiler.Start(ctx); err != nil {
	//  return fmt.Errorf("failed to start profiler: %w", err)
	// }
	// defer pt.profiler.Stop()

	cmd := exec.Command(command, args...)
	g_cmd = cmd

	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(
		os.Environ(),
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

	// Start scanning with high frequency initially
	go func() {
		// Initial high-frequency scanning
		initialTicker := time.NewTicker(1 * time.Millisecond)
		defer initialTicker.Stop()

		// After initial period, reduce frequency slightly
		// time.AfterFunc(5*time.Second, func() {
		//  initialTicker.Stop()
		// })

		// regularTicker := time.NewTicker(10 * time.Millisecond)
		// defer regularTicker.Stop()

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
				// pt.logProcessTree()
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

var DEBUG bool = false
var TRACE bool = false

func (pt *ProcessTracker) LogTrace(format string, args ...any) {
	if TRACE {
		pt.t.Logf(format, args...)
	}
}

func LogDebug(t *testing.T, format string, args ...any) {
	if DEBUG {
		t.Logf(format, args...)
	}
}

var g_RepoInfo *RepoInfo
var g_ConfigManager *diconfig.ReaderConfigManager

func TestExplorationGoDI(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock(), "Failed to remove memlock limit")
	if features.HaveMapType(ebpf.RingBuf) != nil {
		t.Skip("Ringbuffers not supported on this kernel")
	}

	eventOutputWriter := &explorationEventOutputTestWriter{
		t: t,
	}

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

// RepoInfo holds scanned repository package information
type RepoInfo struct {
	Packages   map[string]bool // Package names found in repo
	RepoPath   string          // Path to the repo
	CommitHash string          // Current commit hash (optional)
}

func ScanRepoPackages(repoPath string) (*RepoInfo, error) {
	info := &RepoInfo{
		Packages: make(map[string]bool),
		RepoPath: repoPath,
	}

	// Get git hash if available
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		if hash, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output(); err == nil {
			info.CommitHash = strings.TrimSpace(string(hash))
		}
	}

	err := filepath.Walk(repoPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip certain directories
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

		// Only process .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files and generated files
		if strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, ".pb.go") {
			return nil
		}

		// Ensure the file is within the repo (not in .cache etc)
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

	// Scan packages after clone/checkout
	info, err := ScanRepoPackages(modulePath)
	require.NoError(t, err, "Failed to scan repo packages")

	// Log the organized package information
	var pkgs []string
	for pkg := range info.Packages {
		if strings.Contains(pkg, "/tmp") {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)

	t.Logf("📦 Found %d packages in protobuf repo:", len(pkgs))

	// Group packages by their top-level directory
	groups := make(map[string][]string)
	for _, pkg := range pkgs {
		parts := strings.SplitN(pkg, "/", 2)
		topLevel := parts[0]
		groups[topLevel] = append(groups[topLevel], pkg)
	}

	// Print grouped packages
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

var explorationTestConfigTemplateText = `
{
    "go-di-exploration-test-service": {
        "{{.ProbeId}}": {
            "id": "{{.ProbeId}}",
            "version": 0,
            "type": "LOG_PROBE",
            "language": "go",
            "where": {
                "typeName": "{{.PackageName}}",
                "methodName": "{{.FunctionName}}"
            },
            "tags": [],
            "template": "Executed {{.PackageName}}.{{.FunctionName}}, it took {@duration}ms",
            "segments": [
                {
                "str": "Executed {{.PackageName}}.{{.FunctionName}}, it took "
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
    }
}
`
