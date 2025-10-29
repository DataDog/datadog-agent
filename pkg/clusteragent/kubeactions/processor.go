// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/go-multierror"
)

// ActionStoreInterface defines the interface for action stores
type ActionStoreInterface interface {
	WasExecuted(key ActionKey) bool
	MarkExecuted(key ActionKey, status, message string, timestamp int64)
	GetRecord(key ActionKey) (ActionRecord, bool)
}

// ActionProcessor processes Kubernetes actions from remote config
type ActionProcessor struct {
	validator *ActionValidator
	registry  *ExecutorRegistry
	store     ActionStoreInterface
	ctx       context.Context
}

// NewActionProcessor creates a new ActionProcessor with the given store
func NewActionProcessor(ctx context.Context, registry *ExecutorRegistry, store ActionStoreInterface) *ActionProcessor {
	return &ActionProcessor{
		validator: NewActionValidator(),
		registry:  registry,
		store:     store,
		ctx:       ctx,
	}
}

// Process processes a remote config update containing Kubernetes actions
func (p *ActionProcessor) Process(configKey string, rawConfig state.RawConfig) error {
	// Parse the actions list from the config
	actionsList := &kubeactions.KubeActionsList{}
	err := json.Unmarshal(rawConfig.Config, &actionsList)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v",
			rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	// Create action key for tracking
	actionKey := ActionKey{
		ID:      rawConfig.Metadata.ID,
		Version: rawConfig.Metadata.Version,
	}

	// Check if this action was already executed
	if p.store.WasExecuted(actionKey) {
		record, _ := p.store.GetRecord(actionKey)
		log.Infof("Action %s was already executed with status: %s", actionKey.String(), record.Status)
		return nil
	}

	// Process all actions in the list
	var processingErrors error
	for i, action := range actionsList.Actions {
		if err := p.processAction(action, i, actionKey); err != nil {
			processingErrors = multierror.Append(processingErrors, err)
		}
	}

	return processingErrors
}

// processAction processes a single action
func (p *ActionProcessor) processAction(action *kubeactions.KubeAction, index int, actionKey ActionKey) error {
	log.Infof("Processing action %d: type=%s, resource=%s/%s",
		index, action.ActionType, action.Resource.Kind, action.Resource.Name)

	// Validate the action
	if err := p.validator.ValidateAction(action); err != nil {
		log.Warnf("Action validation failed: %v", err)
		p.store.MarkExecuted(actionKey, "failed", fmt.Sprintf("validation failed: %v", err), time.Now().Unix())
		return err
	}

	// Execute the action
	result := p.registry.Execute(p.ctx, action)

	// Store the result
	p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix())

	// Log the result
	if result.Status == "success" {
		log.Infof("Action executed successfully: %s", result.Message)
	} else {
		log.Errorf("Action execution failed: %s", result.Message)
		return fmt.Errorf("action execution failed: %s", result.Message)
	}

	return nil
}

// GetStore returns the action store for inspection
func (p *ActionProcessor) GetStore() ActionStoreInterface {
	return p.store
}
