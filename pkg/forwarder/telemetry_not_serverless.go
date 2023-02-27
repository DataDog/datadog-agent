// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package forwarder

import (
	"expvar"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var transactionsIntakeOrchestrator = map[model.K8SResource]*expvar.Int{}

func initOrchestratorExpVars() {
	for _, nodeType := range orchestrator.NodeTypes() {
		transactionsIntakeOrchestrator[nodeType] = &expvar.Int{}
		transaction.TransactionsExpvars.Set(nodeType.String(), transactionsIntakeOrchestrator[nodeType])
	}
	transaction.TransactionsExpvars.Set("OrchestratorManifest", &transactionsOrchestratorManifest)
}

func bumpOrchestratorPayload(nodeType int) {
	e, ok := transactionsIntakeOrchestrator[model.K8SResource(nodeType)]
	if !ok {
		log.Errorf("Unknown NodeType %v, cannot bump expvar", nodeType)
		return
	}
	e.Add(1)
}
