// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysconfigcomponent "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	sysconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// sysinfoParams holds CLI flags for the sysinfo command.
type sysinfoParams struct {
	*command.GlobalParams
	outputJSON bool
}

// makeSysinfoCommand returns the "usm sysinfo" cobra command.
func makeSysinfoCommand(globalParams *command.GlobalParams) *cobra.Command {
	params := &sysinfoParams{GlobalParams: globalParams}

	cmd := &cobra.Command{
		Use:   "sysinfo",
		Short: "Show system information relevant to USM",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(
				runSysinfo,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					SysprobeConfigParams: sysconfigimpl.NewParams(sysconfigimpl.WithSysProbeConfFilePath(params.ConfFilePath),
						sysconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams: log.ForOneShot("SYS-PROBE", "off", false),
				}),
				core.Bundle(),
			)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&params.outputJSON, "json", false, "Output system info as JSON")

	return cmd
}

// ProcessInfo holds information about a running process
type ProcessInfo struct {
	PID     int32  `json:"pid"`
	Name    string `json:"name"`
	Cmdline string `json:"cmdline"`
}

// SystemInfo holds system information relevant to USM
type SystemInfo struct {
	KernelVersion string        `json:"kernel_version"`
	OSType        string        `json:"os_type"`
	Architecture  string        `json:"architecture"`
	Hostname      string        `json:"hostname"`
	Processes     []ProcessInfo `json:"processes"`
}

// runSysinfo is the main implementation of the sysinfo command.
func runSysinfo(_ sysconfigcomponent.Component, params *sysinfoParams) error {
	sysInfo := &SystemInfo{}

	// Get kernel version
	kernelVersion, err := getKernelVersion()
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

	// Get all running processes with command lines (filter out kernel threads)
	processes, err := getProcesses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: unable to list processes: %v\n", err)
	} else {
		sysInfo.Processes = processes
	}

	if params.outputJSON {
		return outputSysinfoJSON(sysInfo)
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
	fmt.Println("PID     | Name                           | Command Line")
	fmt.Println("--------|--------------------------------|--------------------------------------------------")

	for _, p := range info.Processes {
		// Truncate cmdline if too long
		cmdline := p.Cmdline
		if len(cmdline) > 50 {
			cmdline = cmdline[:47] + "..."
		}
		// Truncate name if too long
		name := p.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		fmt.Printf("%-7d | %-30s | %s\n", p.PID, name, cmdline)
	}

	return nil
}

// outputSysinfoJSON encodes the system info as indented JSON.
func outputSysinfoJSON(info *SystemInfo) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(info)
}

// getProcesses reads process information from /proc on Linux
func getProcesses() ([]ProcessInfo, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("process listing only supported on Linux")
	}

	processes := make([]ProcessInfo, 0)

	// Read /proc directory
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}

	for _, entry := range entries {
		// Skip non-directory entries
		if !entry.IsDir() {
			continue
		}

		// Check if directory name is a number (PID)
		pid, err := strconv.ParseInt(entry.Name(), 10, 32)
		if err != nil {
			continue
		}

		// Read process name from /proc/[pid]/comm
		name, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		processName := strings.TrimSpace(string(name))

		// Read command line from /proc/[pid]/cmdline
		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}

		// Convert null-separated cmdline to space-separated string
		cmdline := strings.ReplaceAll(string(cmdlineBytes), "\x00", " ")
		cmdline = strings.TrimSpace(cmdline)

		// Only include processes with command lines (excludes kernel threads)
		if cmdline != "" {
			processes = append(processes, ProcessInfo{
				PID:     int32(pid),
				Name:    processName,
				Cmdline: cmdline,
			})
		}
	}

	// Sort by PID for consistent output
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].PID < processes[j].PID
	})

	return processes, nil
}

// getKernelVersion returns the kernel version string
func getKernelVersion() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("kernel version detection only supported on Linux")
	}

	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "", fmt.Errorf("uname failed: %w", err)
	}

	return unix.ByteSliceToString(u.Release[:]), nil
}
