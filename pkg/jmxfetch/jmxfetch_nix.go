// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build jmx
// +build !windows

package jmxfetch

import (
	"os"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Stop stops the JMXFetch process
func (j *JMXFetch) Stop() error {
	var stopChan chan struct{}

	err := j.cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		return err
	}

	if j.managed {
		stopChan = j.stopped
		close(j.shutdown)
	} else {
		stopChan = make(chan struct{})

		go func() {
			j.Wait()
			close(stopChan)
		}()
	}

	select {
	case <-time.After(time.Millisecond * 500):
		log.Warnf("Jmxfetch did not exit during it's grace period, killing it")
		err = j.cmd.Process.Signal(os.Kill)
		if err != nil {
			log.Warnf("Could not kill jmxfetch: %v", err)
		}
	case <-stopChan:
	}
	return nil

}
