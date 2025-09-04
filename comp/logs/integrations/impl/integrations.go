// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

// Package integrationsimpl implements the integrations component interface
package integrationsimpl

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
)

// Logsintegration is the integrations component implementation
type Logsintegration struct {
	logChan            chan integrations.IntegrationLog
	integrationChan    chan integrations.IntegrationConfig
	log                log.Component
	actionCallback     func() error
	registrationList   map[string]bool
	integrationTimeout time.Duration
	registrationLock   sync.Mutex

	errorBatchingInterval time.Duration
	errorList             map[errorRecord]int
	errorLock             sync.Mutex
}

type errorRecord struct {
	integrationID string
	errorType     string
}

// NewLogsIntegration creates a new integrations instance
func NewLogsIntegration(log log.Component, config configComponent.Component) integrations.Component {
	integrationTimeout := config.GetDuration("logs_config.integrations_logs_timeout")

	return &Logsintegration{
		logChan:               make(chan integrations.IntegrationLog),
		integrationChan:       make(chan integrations.IntegrationConfig),
		log:                   log,
		actionCallback:        func() error { return nil },
		registrationList:      make(map[string]bool),
		integrationTimeout:    integrationTimeout,
		errorBatchingInterval: time.Second * 60,
		errorList:             make(map[errorRecord]int),
	}
}

// RegisterIntegration registers an integration with the integrations component
func (li *Logsintegration) RegisterIntegration(id string, cfg integration.Config) {
	if len(cfg.LogsConfig) == 0 {
		return
	}

	sources, err := ad.CreateSources(cfg)
	if err != nil {
		li.log.Errorf("Failed to create source for %q: %v", cfg.Name, err)
		return
	}

	for _, source := range sources {
		// TODO: integrations should only be allowed to have one IntegrationType config.
		if source.Config.Type == config.IntegrationType {
			if err := li.actionCallback(); err != nil {
				li.log.Errorf("Unable to register integration %s: %v", id, err)
				return
			}

			li.log.Infof("Registering integration %s with source %s", id, source.Config.IntegrationName)

			integrationConfig := integrations.IntegrationConfig{
				IntegrationID: id,
				Source:        source,
			}

			select {
			case li.integrationChan <- integrationConfig:
				li.registrationLock.Lock()
				li.registrationList[id] = true
				li.registrationLock.Unlock()
			case <-time.After(li.integrationTimeout):
				li.log.Errorf("Integration could not be registered due to timeout, dropping all further logs for integration %s", id)
				return
			}

			// We only support one integration log per id
			return
		}
	}
}

// SendLog sends a log to any subscribers
func (li *Logsintegration) SendLog(log, integrationID string) {
	li.registrationLock.Lock()
	if _, ok := li.registrationList[integrationID]; !ok {
		li.registrationLock.Unlock()
		li.recordNoisyError(integrationID, "registration")
		return
	}
	li.registrationLock.Unlock()

	if err := li.actionCallback(); err != nil {
		li.recordNoisyError(integrationID, "logs agent startup")
		return
	}

	integrationLog := integrations.IntegrationLog{
		Log:           log,
		IntegrationID: integrationID,
	}

	select {
	case li.logChan <- integrationLog:
	case <-time.After(li.integrationTimeout):
		li.recordNoisyError(integrationID, "timeout")
	}
}

func (li *Logsintegration) recordNoisyError(integrationID string, errorType string) {
	li.errorLock.Lock()
	defer li.errorLock.Unlock()

	errorRecord := errorRecord{
		integrationID: integrationID,
		errorType:     errorType,
	}
	if len(li.errorList) == 0 {
		go li.transmitBatchedErrors()
	}
	li.errorList[errorRecord]++
}

func (li *Logsintegration) transmitBatchedErrors() {
	time.Sleep(li.errorBatchingInterval)
	li.errorLock.Lock()
	defer li.errorLock.Unlock()

	for errorRecord, count := range li.errorList {
		li.log.Errorf("Integration %s has failed to send %d logs due to %s issues", errorRecord.integrationID, count, errorRecord.errorType)
	}
	li.errorList = make(map[errorRecord]int)
}

// SetActionCallback sets the callback to be called when integration actions are performed.
func (li *Logsintegration) SetActionCallback(callback func() error) {
	li.actionCallback = callback
}

// Subscribe returns the channel that receives logs from integrations. Currently
// the integrations component only supports one subscriber, but can be extended
// later by making a new channel for any number of subscribers.
func (li *Logsintegration) Subscribe() chan integrations.IntegrationLog {
	return li.logChan
}

// SubscribeIntegration returns the channel that receives integration configurations
func (li *Logsintegration) SubscribeIntegration() chan integrations.IntegrationConfig {
	return li.integrationChan
}
