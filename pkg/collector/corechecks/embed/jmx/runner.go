// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmx

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
)

type runner struct {
	jmxfetch *jmxfetch.JMXFetch
	started  bool
}

func (r *runner) initRunner(server dogstatsdServer.Component, logger jmxlogger.Component) {
	r.jmxfetch = jmxfetch.NewJMXFetch(logger)
	r.jmxfetch.LogLevel = config.Datadog.GetString("log_level")
	r.jmxfetch.DSD = server
}

func (r *runner) startRunner() error {

	lifecycleMgmt := true
	err := r.jmxfetch.Start(lifecycleMgmt)
	if err != nil {
		s := jmxStatus.StartupError{LastError: err.Error(), Timestamp: time.Now().Unix()}
		jmxStatus.SetStartupError(s)
		return err
	}
	r.started = true
	return nil
}

func (r *runner) configureRunner(instance, initConfig integration.Data) error {
	if err := r.jmxfetch.ConfigureFromInstance(instance); err != nil {
		return err
	}
	return r.jmxfetch.ConfigureFromInitConfig(initConfig)
}

func (r *runner) stopRunner() error {
	if r.jmxfetch != nil && r.started {
		return r.jmxfetch.Stop()
	}
	return nil
}
