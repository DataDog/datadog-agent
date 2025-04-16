// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package servicetest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	"gopkg.in/zorkian/go-datadog-api.v2"
)

// NOTE: If using these functions for debugging works nicely this time, we can consider moving them
//       into common/ to make them more generally available for future similar situations.

// getSystemMemoryMetrics executes the Agent memory check to retrieve system memory metrics from the Datadog Agent on a Windows host.
func getSystemMemoryMetrics(host *components.RemoteHost) ([]datadog.Metric, error) {
	// Run agent check to get system memory usage
	out, err := host.Execute(`& "C:/Program Files/Datadog/Datadog Agent/bin/agent.exe" check memory --json`)
	if err != nil {
		return nil, fmt.Errorf("failed to get system memory usage: %s", err)
	}

	// Skip lines until the JSON array starts
	// TODO(WINA-1363): there is non JSON output before the JSON array, we should fix this in the agent
	lines := strings.Split(out, "\n")
	startIndex := 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			startIndex = i
			break
		}
	}
	out = strings.Join(lines[startIndex:], "\n")

	// convert from JSON
	type aggregatorValues struct {
		Metrics []datadog.Metric `json:"metrics,omitempty"`
	}
	type checkOutput struct {
		Aggregator aggregatorValues `json:"aggregator,omitempty"`
	}
	var checkruns []checkOutput
	err = json.Unmarshal([]byte(out), &checkruns)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal system memory usage: %s", err)
	}
	if len(checkruns) == 0 {
		return nil, fmt.Errorf("no check runs found in output: %s", out)
	}

	return checkruns[0].Aggregator.Metrics, nil
}

// putTopProcessMemoryMetricsPowerShell writes a PowerShell script to the host to get the top process memory metrics from the Datadog Agent on a Windows host.
func putTopProcessMemoryMetricsPowerShellIfNotExist(host *components.RemoteHost, path string) error {
	if exists, _ := host.FileExists(path); exists {
		return nil
	}
	_, err := host.WriteFile(path, []byte(`
function Get-ProcessMemoryUsage {
	# https://learn.microsoft.com/en-us/dotnet/api/system.diagnostics.process?view=net-8.0
	# TODO: This might be wrongly including shared memory via the PagedMemorySize64 property
	Get-Process |
		Sort -Property {$_.PagedMemorySize64-$_.WorkingSet64} -Desc |
		Select Name, PagedMemorySize64, WorkingSet64 -First 10  |
		foreach-object {[PSCustomObject]@{
				Name=$_.Name;
				PagedMemorySize64=$_.PagedMemorySize64;
				WorkingSet64=$_.WorkingSet64;
		}}
}

Get-ProcessMemoryUsage | ConvertTo-JSON
`))

	return err
}

// getTopProcessMemoryMetrics runs a PowerShell script to get the top process memory metrics from the Datadog Agent on a Windows host.
// Sends the system.process.mem.rss and system.process.mem.vms metrics to Datadog.
func getTopProcessMemoryMetrics(host *components.RemoteHost) ([]datadog.Metric, error) {
	metrics := []datadog.Metric{}

	// write the PowerShell script to the host
	// We use a script instead of running the command directly to avoid cluttering the log file/screen every time it's run.
	scriptPath := `C:\topprocessmemorymetrics.ps1`
	err := putTopProcessMemoryMetricsPowerShellIfNotExist(host, scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to write PowerShell script: %s", err)
	}

	// Run script to get process memory usage
	timestamp := (float64)(time.Now().Unix())
	out, err := host.Execute(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get process memory usage: %s", err)
	}

	// unmarshal JSON
	type processMemory struct {
		Name              string  `json:"Name"`
		PagedMemorySize64 float64 `json:"PagedMemorySize64"`
		WorkingSet64      float64 `json:"WorkingSet64"`
	}
	var memoryInfo []processMemory
	err = json.Unmarshal([]byte(out), &memoryInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal process memory usage: %s", err)
	}

	// Create metrics for each process
	for _, proc := range memoryInfo {
		proctags := []string{fmt.Sprintf("process_name:%s", proc.Name)}
		metrics = append(metrics, datadog.Metric{
			Metric: datadog.String("system.processes.mem.rss"),
			Points: []datadog.DataPoint{
				{&timestamp, datadog.Float64(proc.WorkingSet64)},
			},
			Type: datadog.String("gauge"),
			Tags: proctags,
			Host: datadog.String(host.Address),
		})
		metrics = append(metrics, datadog.Metric{
			Metric: datadog.String("system.processes.mem.vms"),
			Points: []datadog.DataPoint{
				{&timestamp, datadog.Float64(proc.PagedMemorySize64)},
			},
			Type: datadog.String("gauge"),
			Tags: proctags,
			Host: datadog.String(host.Address),
		})
	}

	return metrics, nil
}
