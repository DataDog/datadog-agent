// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers/channel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logsScheduler is the current logs-agent scheduler managing the log-source for
// the serverless agent's `chan *ChannelMessaage`.
var logsScheduler *channel.Scheduler

// SetupLogAgent sets up the logs agent to handle messages on the given channel.
func SetupLogAgent(logChannel chan *config.ChannelMessage, sourceName string, source string) (logsAgent.ServerlessLogsAgent, error) {
	agent := logsAgent.NewServerlessLogsAgent()
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

func GetLogsTags() []string {
	if logsScheduler != nil {
		return logsScheduler.GetLogsTags()
	}
	return nil
}
