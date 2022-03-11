// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers/channel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logsScheduler is the current logs-agent scheduler managing the log-source for
// serverless's messages
var logsScheduler *channel.Scheduler

// SetupLogAgent sets up the logs agent to handle messages on the given channel.
func SetupLogAgent(logChannel chan *config.ChannelMessage) {
	agent, err := logs.StartServerless(
		func() *autodiscovery.AutoConfig { return common.AC },
	)
	if err != nil {
		log.Error("Could not start an instance of the Logs Agent:", err)
		return
	}

	logsScheduler = channel.NewScheduler("AWS Logs", "lambda", logChannel, nil)
	agent.AddScheduler(logsScheduler)
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
