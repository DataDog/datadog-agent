// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package pipeline

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MockPipelineChans initializes pipelinesChans for testing purpose
func (pp *Provider) MockPipelineChans() {
	pp.pipelinesChans = [](chan message.Message){}
	pp.pipelinesChans = append(pp.pipelinesChans, make(chan message.Message))
	pp.numberOfPipelines = 1
}
