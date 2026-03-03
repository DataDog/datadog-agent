// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package servicetest

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// queryProcessesWithActiveIo finds which processes are actively generating I/O.
func queryProcessesWithActiveIo(host *components.RemoteHost) (string, error) {
	return host.Execute(`
		Get-Counter -Counter '\Process(*)\IO Data Operations/sec' |
		Where-Object { $_.Countersamples.CookedValue -gt 0 } |
		Select-Object -ExpandProperty countersamples |
		Where-Object { $_.InstanceName -ne "_Total" -and $_.CookedValue -gt 0 } |
		Sort-Object CookedValue -Descending | Format-Table -AutoSize`)
}

// queryDiskQueueLength finds the length of the current disk queue.
// A length more than 10 means there is an I/O backlog.
func queryDiskQueueLength(host *components.RemoteHost) (string, error) {
	return host.Execute(
		`Get-Counter -Counter '\LogicalDisk(C:)\Current Disk Queue Length' -SampleInterval 2 -MaxSamples 3`)
}

// queryAllHandleCounts fetches the handle counts for all processes.
func queryAllHandleCounts(host *components.RemoteHost) (string, error) {
	return host.Execute(`Get-Process | Select-Object ProcessName, Handles`)
}
