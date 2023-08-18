// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && serverless

package otlp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func checkAndUpdateCfg(pcfg PipelineConfig, logsAgentChannel chan *message.Message) error {
	if pcfg.LogsEnabled {
		pipelineError.Store(fmt.Errorf("Cannot enable OTLP log ingestion for serverless"))
		return pipelineError.Load()
	}
	return nil
}
