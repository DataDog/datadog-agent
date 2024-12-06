// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && !serverless

package otlp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func checkAndUpdateCfg(_ config.Component, pcfg PipelineConfig, logsAgentChannel chan *message.Message) error {
	if pcfg.LogsEnabled && logsAgentChannel == nil {
		pipelineError.Store(fmt.Errorf("OTLP logs is enabled but logs agent is not enabled"))
		return pipelineError.Load()
	}
	return nil
}
