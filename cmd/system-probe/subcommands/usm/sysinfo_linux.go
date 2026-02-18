// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	sysconfigcomponent "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	defaultMaxCmdlineLength = 50
	defaultMaxNameLength    = 25
	defaultMaxServiceLength = 20
	maxLanguageLength       = 12 // Maximum width for language column to maintain table alignment

	// Truncation suffix
	truncationSuffix       = "..."
	truncationSuffixLength = 3

	// Default display value for missing data
	missingValuePlaceholder = "-"

	// Service context prefix
	processContextPrefix = "process_context:"
)

// makeSysinfoCommand returns the "usm sysinfo" cobra command.
func makeSysinfoCommand(globalParams *command.GlobalParams) *cobra.Command {
	var maxCmdlineLength int
	var maxNameLength int
	var maxServiceLength int

	cmd := makeOneShotCommand(
		globalParams,
		"sysinfo",
		"Show system information relevant to USM",
		func(sysprobeconfig sysconfigcomponent.Component, params *command.GlobalParams) error {
			return runSysinfoWithConfig(sysprobeconfig, params, maxCmdlineLength, maxNameLength, maxServiceLength)
		},
	)

	cmd.Flags().IntVar(&maxCmdlineLength, "max-cmdline-length", defaultMaxCmdlineLength,
		"Maximum command line length to display (0 for unlimited)")
	cmd.Flags().IntVar(&maxNameLength, "max-name-length", defaultMaxNameLength,
		"Maximum process name length to display (0 for unlimited)")
	cmd.Flags().IntVar(&maxServiceLength, "max-service-length", defaultMaxServiceLength,
		"Maximum service name length to display (0 for unlimited)")

	return cmd
}

// ProcessInfo combines process information with detected language
type ProcessInfo struct {
	Process  *procutil.Process
	Language languagemodels.Language
}

// SystemInfo holds system information relevant to USM
type SystemInfo struct {
	KernelVersion string
	OSType        string
	Architecture  string
	Hostname      string
	Processes     []*ProcessInfo
}

// runSysinfoWithConfig is the main implementation of the sysinfo command with configuration.
func runSysinfoWithConfig(sysprobeconfig sysconfigcomponent.Component, _ *command.GlobalParams, maxCmdlineLength, maxNameLength, maxServiceLength int) error {
	sysInfo := &SystemInfo{}

	// Get kernel version using existing utility
	kernelVersion, err := kernel.Release()
	if err != nil {
		sysInfo.KernelVersion = fmt.Sprintf("<unable to detect: %v>", err)
	} else {
		sysInfo.KernelVersion = kernelVersion
	}

	// Get OS and architecture
	sysInfo.OSType = runtime.GOOS
	sysInfo.Architecture = runtime.GOARCH

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		sysInfo.Hostname = fmt.Sprintf("<unable to detect: %v>", err)
	} else {
		sysInfo.Hostname = hostname
	}

	// Get processes using procutil (same as process-agent)
	probe := procutil.NewProcessProbe()
	defer probe.Close()

	procs, err := probe.ProcessesByPID(time.Now(), false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: unable to list processes: %v\n", err)
		return outputSysinfoHumanReadable(sysInfo, maxCmdlineLength, maxNameLength, maxServiceLength)
	}

	if len(procs) == 0 {
		sysInfo.Processes = []*ProcessInfo{} // Explicit empty slice
		return outputSysinfoHumanReadable(sysInfo, maxCmdlineLength, maxNameLength, maxServiceLength)
	}

	// Convert map to sorted slice by PID
	procList := make([]*procutil.Process, 0, len(procs))
	for _, proc := range procs {
		procList = append(procList, proc)
	}
	sort.Slice(procList, func(i, j int) bool {
		return procList[i].Pid < procList[j].Pid
	})

	// Detect languages using two-phase detection strategy
	languages := detectProcessLanguages(procList, sysprobeconfig)

	// Extract service information for all processes
	serviceExtractor := parser.NewServiceExtractor(true, false, true)
	serviceExtractor.Extract(procs)

	// Populate Service field for each process
	for _, proc := range procList {
		var svc *procutil.Service

		// First priority: DD_SERVICE from process environment (not command-line)
		if ddService := readDDServiceFromEnviron(proc.Pid); ddService != "" {
			svc = &procutil.Service{
				DDService: ddService,
			}
		}

		// Second priority: Generated service name from ServiceExtractor
		// (handles DD_SERVICE from command-line and generates names)
		if serviceContext := serviceExtractor.GetServiceContext(proc.Pid); len(serviceContext) > 0 {
			serviceName := strings.TrimPrefix(serviceContext[0], processContextPrefix)
			if svc == nil {
				svc = &procutil.Service{}
			}
			svc.GeneratedName = serviceName
		}

		if svc != nil {
			proc.Service = svc
		}
	}

	// Combine processes with their detected languages
	sysInfo.Processes = make([]*ProcessInfo, len(procList))
	for i, proc := range procList {
		sysInfo.Processes[i] = &ProcessInfo{
			Process:  proc,
			Language: languages[i],
		}
	}

	return outputSysinfoHumanReadable(sysInfo, maxCmdlineLength, maxNameLength, maxServiceLength)
}

// truncateString truncates a string to maxLength, adding "..." suffix if truncated.
// If maxLength is 0 or negative, no truncation is performed.
func truncateString(s string, maxLength int) string {
	if maxLength <= 0 || len(s) <= maxLength {
		return s
	}
	if maxLength <= truncationSuffixLength {
		return s[:maxLength]
	}
	return s[:maxLength-truncationSuffixLength] + truncationSuffix
}

// detectProcessLanguages performs two-phase language detection for the given processes.
//
// Phase 1: Standard detection via command-line parsing (Python, Node, Ruby, PHP, etc.)
//
//	and optionally RPC to system-probe if language_detection.enabled is set.
//	Does NOT require elevated privileges for command-line detection.
//
// Phase 2: Privileged detection fallback for any processes that remain Unknown.
//
//	Performs direct binary analysis to detect Go, .NET, and processes with
//	tracer/injector metadata. Requires elevated privileges (CAP_PTRACE or root).
//
// Returns a slice of detected languages corresponding to the input processes.
func detectProcessLanguages(procList []*procutil.Process, sysprobeconfig sysconfigcomponent.Component) []languagemodels.Language {
	// Phase 1: Standard language detection
	languageProcs := make([]languagemodels.Process, len(procList))
	for i, p := range procList {
		languageProcs[i] = p
	}

	languagesPtr := languagedetection.DetectLanguage(languageProcs, sysprobeconfig.Object())

	// Phase 2: Privileged detection fallback
	// Build list of unknown processes and convert pointer results to values
	languages := make([]languagemodels.Language, len(procList))
	unknownIndices := make([]int, 0, len(procList))
	unknownProcs := make([]languagemodels.Process, 0, len(procList))

	for i, langPtr := range languagesPtr {
		if langPtr != nil {
			languages[i] = *langPtr
			// Check if we need privileged fallback for this process
			if langPtr.Name == "" || langPtr.Name == languagemodels.Unknown {
				unknownIndices = append(unknownIndices, i)
				unknownProcs = append(unknownProcs, procList[i])
			}
		} else {
			// Nil pointer - treat as unknown and try privileged detection
			unknownIndices = append(unknownIndices, i)
			unknownProcs = append(unknownProcs, procList[i])
		}
	}

	// Apply privileged detection to unknown processes
	if len(unknownProcs) > 0 {
		detector := privileged.NewLanguageDetector()
		privilegedLangs := detector.DetectWithPrivileges(unknownProcs)
		for i, idx := range unknownIndices {
			languages[idx] = privilegedLangs[i]
		}
	}

	return languages
}

// formatLanguage formats a language for display
func formatLanguage(lang languagemodels.Language) string {
	if lang.Name == "" {
		return missingValuePlaceholder
	}
	if lang.Version != "" {
		return fmt.Sprintf("%s/%s", lang.Name, lang.Version)
	}
	return string(lang.Name)
}

// outputSysinfoHumanReadable prints system info in a text-based format.
func outputSysinfoHumanReadable(info *SystemInfo, maxCmdlineLength, maxNameLength, maxServiceLength int) error {
	fmt.Println("=== USM System Information ===")
	fmt.Println()
	fmt.Printf("Kernel Version: %s\n", info.KernelVersion)
	fmt.Printf("OS Type:        %s\n", info.OSType)
	fmt.Printf("Architecture:   %s\n", info.Architecture)
	fmt.Printf("Hostname:       %s\n", info.Hostname)
	fmt.Println()

	fmt.Printf("Running Processes: %d\n", len(info.Processes))
	fmt.Println()
	fmt.Println("PID     | PPID    | Name                      | Service              | Language     | Command")
	fmt.Println("--------|---------|---------------------------|----------------------|--------------|--------------------------------------------------")

	for _, procInfo := range info.Processes {
		proc := procInfo.Process

		// Truncate and format fields
		name := truncateString(proc.Name, maxNameLength)
		cmdline := truncateString(formatCmdline(proc.Cmdline), maxCmdlineLength)
		langStr := truncateString(formatLanguage(procInfo.Language), maxLanguageLength)

		// Format the service name from Process.Service field
		// Prioritize DDService (from DD_SERVICE env var) over GeneratedName
		serviceStr := missingValuePlaceholder
		if proc.Service != nil {
			if proc.Service.DDService != "" {
				serviceStr = proc.Service.DDService
			} else if proc.Service.GeneratedName != "" {
				serviceStr = proc.Service.GeneratedName
			}
		}
		serviceStr = truncateString(serviceStr, maxServiceLength)

		fmt.Printf("%-7d | %-7d | %-25s | %-20s | %-12s | %s\n", proc.Pid, proc.Ppid, name, serviceStr, langStr, cmdline)
	}

	return nil
}

// formatCmdline joins cmdline args into a single string
func formatCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, arg := range args {
		if i > 0 {
			builder.WriteString(" ")
		}
		builder.WriteString(arg)
	}
	return builder.String()
}

// readDDServiceFromEnviron reads DD_SERVICE from /proc/<pid>/environ
// This is a lightweight direct reader that avoids heavy service discovery imports.
// It looks for DD_SERVICE first, then falls back to parsing DD_TAGS for "service:" prefix.
func readDDServiceFromEnviron(pid int32) string {
	environPath := fmt.Sprintf("/proc/%d/environ", pid)
	data, err := os.ReadFile(environPath)
	if err != nil {
		return ""
	}

	// Environment variables are null-terminated in /proc/<pid>/environ
	envVars := strings.Split(string(data), "\x00")

	// First priority: DD_SERVICE
	for _, env := range envVars {
		if strings.HasPrefix(env, "DD_SERVICE=") {
			service := strings.TrimPrefix(env, "DD_SERVICE=")
			if service != "" {
				return service
			}
		}
	}

	// Second priority: service: tag in DD_TAGS
	for _, env := range envVars {
		if strings.HasPrefix(env, "DD_TAGS=") {
			tags := strings.TrimPrefix(env, "DD_TAGS=")
			// Parse comma-separated tags
			for _, tag := range strings.Split(tags, ",") {
				if strings.HasPrefix(tag, "service:") {
					service := strings.TrimPrefix(tag, "service:")
					if service != "" {
						return service
					}
				}
			}
		}
	}

	return ""
}
