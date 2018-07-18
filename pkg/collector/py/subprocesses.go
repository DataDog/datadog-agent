// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package py

import (
	"errors"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var runningProcesses *cache.BasicCache = cache.NewBasicCache()

// TerminateRunningProcesses attempts to terminate all tracked running processes gracefully
func TerminateRunningProcesses() error {
	var termErr error
	procs := runningProcesses.Items()
	for _, p := range procs {
		current := p.(*exec.Cmd)
		if current.Process != nil {
			log.Debugf("Terminating subprocess with pid: (%v)", current.Process.Pid)
			err := subprocessEnd(current.Process)
			if err != nil {
				log.Debugf("Unable to gracefully shutdown process: %v", err)
				if termErr == nil {
					termErr = errors.New("Unable in attept to terminate all processes gracefully")
				}
			}
		}
	}

	return termErr
}
