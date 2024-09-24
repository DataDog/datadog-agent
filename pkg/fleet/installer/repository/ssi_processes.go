// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package repository

import (
	"bytes"
	"slices"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/olekukonko/tablewriter"
	"github.com/shirou/gopsutil/v3/process"
)

// InjectedProcess repressents a running process that has Single Step Instrumentation
// loaded and has been isntrumented.
type InjectedProcess struct {
	Pid int

	ServiceName    string
	LanguageName   string
	RuntimeName    string
	RuntimeVersion string

	LibraryVersion  string
	InjectorVersion string

	IsInjected      bool
	InjectionStatus string
	Reason          string
}

var tableHeader = []string{
	"PID", "SERVICE_NAME", "LANGUAGE_NAME", "RUNTIME_NAME", "RUNTIME_VERSION",
	"LIBRARY_VERSION", "INJECTOR_VERSION", "IS_INJECTED", "INJECTION_STATUS", "REASON",
}

// ListSSIProcesses returns the list of running processes using Single Step Instrumentation
func ListSsiProcesses() ([]InjectedProcess, error) {
	injectedProcesses := []InjectedProcess{}
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}
	for _, pid := range pids {
		p := process.Process{
			Pid: pid,
		}
		injectedProcess, found, err := getSsiProcess(&p)
		if err != nil {
			log.Errorf("list Single-Step-Injection enabled processes pid=%v : %v", pid, err)
		}
		if !found {
			continue
		}
		injectedProcesses = append(injectedProcesses, injectedProcess)
	}
	return injectedProcesses, nil
}

// FormatInjectedProcesses formats a liat of injected processes to a table
func FormatInjectedProcesses(injectedProcesses []InjectedProcess) string {
	slices.SortFunc(injectedProcesses, func(l InjectedProcess, r InjectedProcess) int {
		if l.Pid < r.Pid {
			return -1
		} else if l.Pid == r.Pid {
			return 0
		}
		return 1
	})

	var buffer bytes.Buffer

	// plain table with no borders
	table := tablewriter.NewWriter(&buffer)

	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t")

	table.SetHeader(tableHeader)
	for _, process := range injectedProcesses {
		table.Append([]string{
			strconv.Itoa(process.Pid),
			process.ServiceName,
			process.LanguageName,
			process.RuntimeName,
			process.RuntimeVersion,
			process.LibraryVersion,
			process.InjectorVersion,
			strconv.FormatBool(process.IsInjected),
			process.InjectionStatus,
			process.Reason,
		})
	}
	table.Render()
	return buffer.String()
}
