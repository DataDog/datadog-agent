// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build jmx

package jmx

import (
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/DataDog/datadog-agent/pkg/status"
)

type runner struct {
	jmxfetch *jmxfetch.JMXFetch
	started  bool
}

func (r *runner) initRunner() {
	r.jmxfetch = &jmxfetch.JMXFetch{}
	r.jmxfetch.LogLevel = config.Datadog.GetString("log_level")
}

func (r *runner) startRunner() error {

	lifecycleMgmt := true
	if runtime.GOOS == "windows" {
		lifecycleMgmt = false
	}

	err := r.jmxfetch.Start(lifecycleMgmt)
	if err != nil {
		s := status.JMXStartupError{err.Error(), time.Now().Unix()}
		status.SetJMXStartupError(s)
		return err
	}
	r.started = true
	return nil
}

func (r *runner) configureRunner(instance, initConfig integration.Data) error {
	if err := r.jmxfetch.ConfigureFromInstance(instance); err != nil {
		return err
	}
	if err := r.jmxfetch.ConfigureFromInitConfig(initConfig); err != nil {
		return err
	}
	return nil
}

func (r *runner) stopRunner() error {
	if r.jmxfetch != nil && r.started {
		return r.jmxfetch.Stop()
	}
	return nil
}
