// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package aggregator

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orch "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
)

func buildOrchestratorForwarder() defaultforwarder.Forwarder {
	return orch.NewOrchestratorForwarder()
}
