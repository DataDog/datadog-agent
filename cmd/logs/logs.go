// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package main

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/container"
	"github.com/DataDog/datadog-agent/pkg/logs/input/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/input/tailer"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Start starts the forwarder
func Start() {

	cm := sender.NewConnectionManager(
		config.LogsAgent.GetString("log_dd_url"),
		config.LogsAgent.GetInt("log_dd_port"),
		config.LogsAgent.GetBool("skip_ssl_validation"),
	)

	auditorChan := make(chan message.Message, config.ChanSizes)
	a := auditor.New(auditorChan)
	a.Start()

	pp := pipeline.NewProvider()
	pp.Start(cm, auditorChan)

	l := listener.New(config.GetLogsSources(), pp)
	l.Start()

	s := tailer.New(config.GetLogsSources(), pp, a)
	s.Start()

	c := container.New(config.GetLogsSources(), pp, a)
	c.Start()
}
