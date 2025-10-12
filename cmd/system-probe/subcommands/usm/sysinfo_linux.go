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
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// makeSysinfoCommand returns the "usm sysinfo" cobra command.
func makeSysinfoCommand(globalParams *command.GlobalParams) *cobra.Command {
	return makeOneShotCommand(
		globalParams,
		"sysinfo",
		"Show system information relevant to USM",
		runSysinfo,
	)
}

// SystemInfo holds system information relevant to USM
type SystemInfo struct {
	KernelVersion string
	OSType        string
	Architecture  string
	Hostname      string
	Processes     []*procutil.Process
}

// runSysinfo is the main implementation of the sysinfo command.
func runSysinfo(_ sysconfigcomponent.Component, params *cmdParams) error {
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
		// Convert map to slice
		procList := make([]*procutil.Process, 0, len(procs))
		for _, proc := range procs {
			procList = append(procList, proc)
		}

		// Sort by PID for consistent output
		sort.Slice(procList, func(i, j int) bool {
			return procList[i].Pid < procList[j].Pid
		})

		sysInfo.Processes = procList
	}

	if params.outputJSON {
		// Create simplified output with only essential process fields
		output := map[string]interface{}{
			"kernel_version": sysInfo.KernelVersion,
			"os_type":        sysInfo.OSType,
			"architecture":   sysInfo.Architecture,
			"hostname":       sysInfo.Hostname,
			"processes":      simplifyProcesses(sysInfo.Processes),
		}
		return outputJSON(output)
	}

	return outputSysinfoHumanReadable(sysInfo)
}

// outputSysinfoHumanReadable prints system info in a text-based format.
func outputSysinfoHumanReadable(info *SystemInfo) error {
	fmt.Println("=== USM System Information ===")
	fmt.Println()
	fmt.Printf("Kernel Version: %s\n", info.KernelVersion)
	fmt.Printf("OS Type:        %s\n", info.OSType)
	fmt.Printf("Architecture:   %s\n", info.Architecture)
	fmt.Printf("Hostname:       %s\n", info.Hostname)
	fmt.Println()

	fmt.Printf("Running Processes: %d\n", len(info.Processes))
	fmt.Println()
	fmt.Println("PID     | PPID    | Name                      | Command")
	fmt.Println("--------|---------|---------------------------|--------------------------------------------------")

	for _, p := range info.Processes {
		// Truncate fields if too long
		name := p.Name
		if len(name) > 25 {
			name = name[:22] + "..."
		}
		cmdline := formatCmdline(p.Cmdline)
		if len(cmdline) > 50 {
			cmdline = cmdline[:47] + "..."
		}
		fmt.Printf("%-7d | %-7d | %-25s | %s\n", p.Pid, p.Ppid, name, cmdline)
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

// simplifyProcesses extracts only the essential fields from processes for JSON output
func simplifyProcesses(procs []*procutil.Process) []map[string]interface{} {
	simplified := make([]map[string]interface{}, len(procs))
	for i, p := range procs {
		simplified[i] = map[string]interface{}{
			"pid":     p.Pid,
			"ppid":    p.Ppid,
			"name":    p.Name,
			"cmdline": p.Cmdline,
		}
	}
	return simplified
}
