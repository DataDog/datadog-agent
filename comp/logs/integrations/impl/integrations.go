// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package integrationsimpl implements the integrations component interface
package integrationsimpl

import (
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

type logsintegration struct {
	logChan chan integrations.IntegrationLog
}

// NewComponent creates a new integrations component
func NewComponent() integrations.Component {
	return &logsintegration{
		logChan: make(chan integrations.IntegrationLog),
	}
}

// SendLog sends a log to any subscribers
func (li *logsintegration) SendLog(log, integrationID string) {
	integrationLog := integrations.IntegrationLog{
		Log:           log,
		IntegrationID: integrationID,
	}

	li.logChan <- integrationLog
}

// Subscribe returns the channel that receives logs from integrations. Currently
// the integrations component only supports one subscriber, but can be extended
// later by making a new channel for any number of subscribers.
func (li *logsintegration) Subscribe() chan integrations.IntegrationLog {
	return li.logChan
}
