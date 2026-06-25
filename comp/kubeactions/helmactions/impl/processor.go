// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ActionProcessor processes helm actions received from remote config.
type ActionProcessor struct {
	store    ActionStoreInterface
	reporter *ResultReporter
	ctx      context.Context
}

// NewActionProcessor creates a new ActionProcessor.
func NewActionProcessor(ctx context.Context, store ActionStoreInterface, reporter *ResultReporter) *ActionProcessor {
	return &ActionProcessor{
		store:    store,
		reporter: reporter,
		ctx:      ctx,
	}
}

// Process handles a single remote config update.
func (p *ActionProcessor) Process(configKey string, rawConfig state.RawConfig) error {
	if rawConfig.Metadata.ID == "" {
		return errors.New("action metadata.id is missing")
	}
	if rawConfig.Metadata.Version == 0 {
		return fmt.Errorf("action %s metadata.version is missing or zero", rawConfig.Metadata.ID)
	}

	var actionsList HelmActionsList
	if err := json.Unmarshal(rawConfig.Config, &actionsList); err != nil {
		log.Errorf("[HelmActions] Failed to unmarshal config %s (id=%s, version=%d): %v",
			configKey, rawConfig.Metadata.ID, rawConfig.Metadata.Version, err)
		return fmt.Errorf("failed to unmarshal config id:%s version:%d key:%s: %w",
			rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	if len(actionsList.Actions) != 1 {
		err := fmt.Errorf("expected exactly 1 action per config, got %d", len(actionsList.Actions))
		log.Errorf("[HelmActions] Rejecting config %s: %v", configKey, err)
		return err
	}

	actionKey := ActionKey{
		ID:      rawConfig.Metadata.ID,
		Version: rawConfig.Metadata.Version,
	}
	action := actionsList.Actions[0]
	receivedAt := time.Now().Unix()

	if !p.store.Claim(actionKey) {
		record, _ := p.store.GetRecord(actionKey)
		log.Debugf("[HelmActions] Action %s already processed with status: %s", actionKey, record.Status)
		if record.Status == StatusFailed || record.Status == StatusExpired {
			return fmt.Errorf("action previously %s: %s", record.Status, record.Message)
		}
		return nil
	}

	orgID := parseOrgIDFromConfigKey(configKey)
	_ = orgID // reserved for future EVP reporting

	return p.processAction(action, actionKey, receivedAt)
}

func (p *ActionProcessor) processAction(action *HelmAction, actionKey ActionKey, receivedAt int64) error {
	actionType := GetActionType(action)
	log.Infof("[HelmActions] Processing action %s (type=%s)", actionKey, actionType)

	p.reporter.ReportReceived(actionKey, action)

	// Validate timestamp
	if action.Timestamp == 0 {
		result := ExecutionResult{Status: StatusFailed, Message: "timestamp is missing"}
		p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, 0)
		p.reporter.ReportResult(actionKey, action, result)
		log.Errorf("[HelmActions] Action %s rejected: %s", actionKey, result.Message)
		return errors.New("action timestamp is missing")
	}

	actionCreatedAt := action.Timestamp
	ts := time.Unix(actionCreatedAt, 0)
	if err := ValidateTimestamp(ts); err != nil {
		result := ExecutionResult{Status: StatusExpired, Message: fmt.Sprintf("timestamp validation failed: %v", err)}
		p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, actionCreatedAt)
		p.reporter.ReportResult(actionKey, action, result)
		log.Errorf("[HelmActions] Action %s rejected: %s", actionKey, result.Message)
		return err
	}

	// Execute
	result := ExecutionResult{} // TODO // p.registry.Execute(p.ctx, action)
	executedAt := time.Now().Unix()
	p.store.MarkExecuted(actionKey, result.Status, result.Message, executedAt, receivedAt, actionCreatedAt)
	p.reporter.ReportResult(actionKey, action, result)

	if result.Status == StatusSuccess {
		log.Infof("[HelmActions] Action %s executed: %s", actionKey, result.Message)
		return nil
	}
	log.Errorf("[HelmActions] Action %s failed: %s", actionKey, result.Message)
	return fmt.Errorf("action execution failed: %s", result.Message)
}

// GetStore returns the action store for inspection.
func (p *ActionProcessor) GetStore() ActionStoreInterface {
	return p.store
}

// parseOrgIDFromConfigKey extracts the org ID from an RC config key path.
// Config keys have the format: datadog/<org_id>/<product>/<config_id>/<file>
func parseOrgIDFromConfigKey(configKey string) int64 {
	parts := strings.SplitN(configKey, "/", 4)
	if len(parts) < 2 || parts[1] == "" {
		return 0
	}
	orgID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0
	}
	return orgID
}
