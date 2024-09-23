// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

package repository

import "github.com/shirou/gopsutil/v3/process"

func getSsiProcess(p *process.Process) (InjectedProcess, bool, error) {
	return InjectedProcess{}, false, nil
}
