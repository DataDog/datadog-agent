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
	"runtime"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysconfigcomponent "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	sysconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

// ProcessInfo holds basic information about a process
type ProcessInfo struct {
	PID     int32  `json:"pid"`
	PPID    int32  `json:"ppid"`
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
		// Convert to slice
		procList := make([]ProcessInfo, 0, len(procs))
		for _, proc := range procs {
			procList = append(procList, ProcessInfo{
				PID:     proc.Pid,
				PPID:    proc.Ppid,
				Name:    proc.Name,
				Cmdline: formatCmdline(proc.Cmdline),
			})
		}

		// Sort by PID for consistent output
		sort.Slice(procList, func(i, j int) bool {
			return procList[i].PID < procList[j].PID
		})

		sysInfo.Processes = procList
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
	fmt.Println("PID     | PPID    | Name                      | Command")
	fmt.Println("--------|---------|---------------------------|--------------------------------------------------")

	for _, p := range info.Processes {
		// Truncate fields if too long
		name := p.Name
		if len(name) > 25 {
			name = name[:22] + "..."
		}
		cmdline := p.Cmdline
		if len(cmdline) > 50 {
			cmdline = cmdline[:47] + "..."
		}
		fmt.Printf("%-7d | %-7d | %-25s | %s\n", p.PID, p.PPID, name, cmdline)
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

// outputSysinfoJSON encodes the system info as indented JSON.
func outputSysinfoJSON(info *SystemInfo) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(info)
}
