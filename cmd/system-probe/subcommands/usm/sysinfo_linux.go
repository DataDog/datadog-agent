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
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	sysconfigcomponent "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	defaultMaxCmdlineLength = 50
	defaultMaxNameLength    = 25
	maxLanguageLength       = 12 // Maximum width for language column to maintain table alignment
)

// makeSysinfoCommand returns the "usm sysinfo" cobra command.
func makeSysinfoCommand(globalParams *command.GlobalParams) *cobra.Command {
	var maxCmdlineLength int
	var maxNameLength int

	cmd := makeOneShotCommand(
		globalParams,
		"sysinfo",
		"Show system information relevant to USM",
		func(sysprobeconfig sysconfigcomponent.Component, params *command.GlobalParams) error {
			return runSysinfoWithConfig(sysprobeconfig, params, maxCmdlineLength, maxNameLength)
		},
	)

	cmd.Flags().IntVar(&maxCmdlineLength, "max-cmdline-length", defaultMaxCmdlineLength,
		"Maximum command line length to display (0 for unlimited)")
	cmd.Flags().IntVar(&maxNameLength, "max-name-length", defaultMaxNameLength,
		"Maximum process name length to display (0 for unlimited)")

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
func runSysinfoWithConfig(_ sysconfigcomponent.Component, _ *command.GlobalParams, maxCmdlineLength, maxNameLength int) error {
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
	} else {
		// Convert map to sorted slice by PID
		procList := make([]*procutil.Process, 0, len(procs))
		for _, proc := range procs {
			procList = append(procList, proc)
		}
		sort.Slice(procList, func(i, j int) bool {
			return procList[i].Pid < procList[j].Pid
		})

		// Detect languages for all processes
		detector := privileged.NewLanguageDetector()
		languageProcs := make([]languagemodels.Process, len(procList))
		for i, p := range procList {
			languageProcs[i] = p
		}
		languages := detector.DetectWithPrivileges(languageProcs)

		// Combine processes with their detected languages
		sysInfo.Processes = make([]*ProcessInfo, len(procList))
		for i, proc := range procList {
			sysInfo.Processes[i] = &ProcessInfo{
				Process:  proc,
				Language: languages[i],
			}
		}
	}

	return outputSysinfoHumanReadable(sysInfo, maxCmdlineLength, maxNameLength)
}

// formatLanguage formats a language for display
func formatLanguage(lang languagemodels.Language) string {
	if lang.Name == "" {
		return "-"
	}
	if lang.Version != "" {
		return fmt.Sprintf("%s/%s", lang.Name, lang.Version)
	}
	return string(lang.Name)
}

// outputSysinfoHumanReadable prints system info in a text-based format.
func outputSysinfoHumanReadable(info *SystemInfo, maxCmdlineLength, maxNameLength int) error {
	fmt.Println("=== USM System Information ===")
	fmt.Println()
	fmt.Printf("Kernel Version: %s\n", info.KernelVersion)
	fmt.Printf("OS Type:        %s\n", info.OSType)
	fmt.Printf("Architecture:   %s\n", info.Architecture)
	fmt.Printf("Hostname:       %s\n", info.Hostname)
	fmt.Println()

	fmt.Printf("Running Processes: %d\n", len(info.Processes))
	fmt.Println()
	fmt.Println("PID     | PPID    | Name                      | Language     | Command")
	fmt.Println("--------|---------|---------------------------|--------------|--------------------------------------------------")

	for _, procInfo := range info.Processes {
		proc := procInfo.Process

		// Truncate fields based on configuration (0 means unlimited)
		name := proc.Name
		if maxNameLength > 0 && len(name) > maxNameLength {
			name = name[:maxNameLength-3] + "..."
		}
		cmdline := formatCmdline(proc.Cmdline)
		if maxCmdlineLength > 0 && len(cmdline) > maxCmdlineLength {
			cmdline = cmdline[:maxCmdlineLength-3] + "..."
		}

		// Format the detected language
		langStr := formatLanguage(procInfo.Language)
		if len(langStr) > maxLanguageLength {
			langStr = langStr[:maxLanguageLength]
		}

		fmt.Printf("%-7d | %-7d | %-25s | %-12s | %s\n", proc.Pid, proc.Ppid, name, langStr, cmdline)
	}

	return nil
}

// formatCmdline joins cmdline args into a single string
func formatCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		result += arg
	}
	return result
}
