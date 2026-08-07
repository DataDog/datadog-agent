// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logs provides log collection and scheduling for serverless environments.
package logs

import (
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent/def"
	agentimpl "github.com/DataDog/datadog-agent/comp/logs/agent/impl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers/channel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logsScheduler is the current logs-agent scheduler managing the log-source for
// the serverless agent's `chan *ChannelMessaage`.
var logsScheduler *channel.Scheduler

// SetupLogAgent sets up the logs agent to handle messages on the given
// channel. useRegistryAuditor is threaded straight through to
// agentimpl.NewServerlessLogsAgent; see that function's doc comment for why
// callers must choose explicitly.
func SetupLogAgent(logChannel chan *config.ChannelMessage, sourceName string, source string, tagger tagger.Component, compression logscompression.Component, hostname hostnameinterface.Component, useRegistryAuditor bool) (logsAgent.ServerlessLogsAgent, error) {
	agent := agentimpl.NewServerlessLogsAgent(tagger, compression, hostname, useRegistryAuditor)
	err := agent.Start()
	if err != nil {
		log.Error("Could not start an instance of the Logs Agent:", err)
		return nil, err
	}

	logsScheduler = channel.NewScheduler(sourceName, source, logChannel)
	agent.AddScheduler(logsScheduler)
	return agent, nil
}

// SetLogsTags updates the tags attached to logs messages.
//
// This function retains the given tags slice, which must not be modified after this
// call.
func SetLogsTags(tags []string) {
	if logsScheduler != nil {
		logsScheduler.SetLogsTags(tags)
	}
}
