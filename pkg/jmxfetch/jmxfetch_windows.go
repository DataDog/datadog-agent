// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmxfetch

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Stop stops the JMXFetch process
func (j *JMXFetch) Stop() error {
	var stopChan chan struct{}

	if j.cmd.Process != nil {
		err := j.cmd.Process.Kill()
		if err != nil {
			return err
		}
	}

	if j.managed {
		stopChan = j.stopped
		close(j.shutdown)
	} else {
		stopChan = make(chan struct{})

		go func() {
			_ = j.Wait()
			close(stopChan)
		}()
	}

	select {
	case <-time.After(time.Millisecond * 1000):
		log.Warnf("Jmxfetch was still running 1 second after trying to kill it")
	case <-stopChan:
	}
	return nil
}
