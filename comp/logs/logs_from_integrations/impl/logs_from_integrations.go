// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package impl implements the logs_from_integrations component interface
package logs_from_integrationsimpl

import (
	logs_from_integrations "github.com/DataDog/datadog-agent/comp/logs/logs_from_integrations/def"
)

type Requires struct {
}

type Provides struct {
	Comp logs_from_integrations.Component
}

type logsintegration struct {
	logChan chan logs_from_integrations.IntegrationLog
}

// NewComponent creates a new logs_from_integrations component
func NewComponent(reqs Requires) (Provides, error) {
	logsInt := &logsintegration{
		logChan: make(chan logs_from_integrations.IntegrationLog),
	}

	provides := Provides{
		Comp: logsInt,
	}
	return provides, nil
}

// SendLog sends a log to any subscribers
func (li *logsintegration) SendLog(log, integrationID string) {
	integrationLog := &logs_from_integrations.IntegrationLog{
		Log:           log,
		IntegrationID: integrationID,
	}

	// TODO: is this correct
	li.logChan <- *integrationLog
}

// Subscribe returns a channel that sends all logs sent from integrations
func (li *logsintegration) Subscribe() chan logs_from_integrations.IntegrationLog {
	return li.logChan
}
