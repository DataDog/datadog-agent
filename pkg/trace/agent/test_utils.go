// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func agentWithDefaults() (agnt *Agent, stop func()) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	return NewAgent(ctx, cfg), cancelFunc
}
