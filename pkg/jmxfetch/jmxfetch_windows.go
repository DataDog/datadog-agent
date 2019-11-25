// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build jmx

package jmxfetch

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (j *JMXFetch) Monitor() {}

// Stop stops the JMXFetch process
func (j *JMXFetch) Stop() error {
	err := j.cmd.Process.Kill()
	if err != nil {
		return err
	}

	stopChan := make(chan struct{})
	go func() {
		j.Wait()
		close(stopChan)
	}()

	select {
	case <-time.After(time.Millisecond * 1000):
		log.Warnf("Jmxfetch was still running 1 second after trying to kill it")
	case <-stopChan:
	}
	return nil
}
