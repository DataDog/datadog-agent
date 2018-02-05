// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"github.com/spf13/viper"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/container"
	"github.com/DataDog/datadog-agent/pkg/logs/input/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/input/tailer"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

// isRunning indicates whether logs-agent is running or not
var isRunning bool

// Start starts logs-agent
func Start(ddConfig *viper.Viper) error {
	config, err := config.Build(ddConfig)
	if err != nil {
		return err
	}
	go run(config)
	return nil
}

// run sets up the pipeline to process logs and them to Datadog back-end
func run(config *config.Config) {
	isRunning = true

	cm := sender.NewConnectionManager(config)

	auditorChan := make(chan message.Message, config.GetChanSize())
	a := auditor.New(auditorChan, config.GetRunPath())
	a.Start()

	pp := pipeline.NewProvider(config)
	pp.Start(cm, auditorChan)

	sources := config.GetLogsSources()

	l := listener.New(sources.GetValidSources(), pp)
	l.Start()

	s := tailer.New(sources.GetValidSources(), config.GetOpenFilesLimit(), pp, a)
	s.Start()

	c := container.New(sources.GetValidSources(), pp, a)
	c.Start()

	status.Initialize(sources.GetSources())

}

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	if !isRunning {
		return status.Status{IsRunning: false}
	}
	return status.Get()
}
