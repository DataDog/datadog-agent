// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"google.golang.org/protobuf/encoding/protojson"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ActionStoreInterface defines the interface for action stores
type ActionStoreInterface interface {
	Claim(key ActionKey) bool
	MarkExecuted(key ActionKey, status, message string, executedAt int64, receivedAt int64, actionCreatedAt int64)
	GetRecord(key ActionKey) (ActionRecord, bool)
}

// ActionProcessor processes Kubernetes actions from remote-config
type ActionProcessor struct {
	validator *ActionValidator
	registry  *ExecutorRegistry
	store     ActionStoreInterface
	reporter  *ResultReporter
	ctx       context.Context
}

// NewActionProcessor creates a new ActionProcessor with the given store and event platform forwarder
func NewActionProcessor(ctx context.Context, registry *ExecutorRegistry, store ActionStoreInterface, epForwarder eventplatform.Forwarder, clusterName, clusterID string) *ActionProcessor {
	return &ActionProcessor{
		validator: NewActionValidator(),
		registry:  registry,
		store:     store,
		reporter:  NewResultReporter(epForwarder, clusterName, clusterID),
		ctx:       ctx,
	}
}

// Process processes a remote config update containing Kubernetes actions
func (p *ActionProcessor) Process(configKey string, rawConfig state.RawConfig) error {
	// Validate metadata
	if rawConfig.Metadata.ID == "" {
		err := errors.New("action metadata.id is missing")
		log.Errorf("[KubeActions] Rejecting config %s: %v", configKey, err)
		return err
	}
	if rawConfig.Metadata.Version == 0 {
		err := fmt.Errorf("action %s metadata.version is missing or zero", rawConfig.Metadata.ID)
		log.Errorf("[KubeActions] Rejecting config %s: %v", configKey, err)
		return err
	}

	// Parse the actions list from the config
	// NOTE: We use protojson instead of encoding/json because the KubeAction message
	// uses protobuf oneof fields (delete_pod, restart_deployment) which encoding/json
	// cannot properly unmarshal. protojson handles oneof fields correctly.
	actionsList := &kubeactions.KubeActionsList{}
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true, // Ignore unknown fields for forward compatibility
	}
	if err := unmarshaler.Unmarshal(rawConfig.Config, actionsList); err != nil {
		log.Errorf("[KubeActions] Failed to unmarshal config %s (id=%s, version=%d): %v",
			configKey, rawConfig.Metadata.ID, rawConfig.Metadata.Version, err)
		return fmt.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v",
			rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	// Enforce exactly one action per config
	if len(actionsList.Actions) != 1 {
		err := fmt.Errorf("expected exactly 1 action per config, got %d", len(actionsList.Actions))
		log.Errorf("[KubeActions] Rejecting config %s: %v", configKey, err)
		return err
	}

	// Create action key for tracking
	actionKey := ActionKey{
		ID:      rawConfig.Metadata.ID,
		Version: rawConfig.Metadata.Version,
	}
	action := actionsList.Actions[0]

	// Record when we received this action
	receivedAt := time.Now().Unix()

	// Check if we can claim the action
	if !p.store.Claim(actionKey) {
		record, _ := p.store.GetRecord(actionKey)
		log.Debugf("[KubeActions] Action %s was already processed with status: %s", actionKey.String(), record.Status)
		if record.Status == StatusFailed || record.Status == StatusExpired {
			return fmt.Errorf("action previously %s: %s", record.Status, record.Message)
		}
		return nil
	}

	// Extract org ID from the config key path
	orgID := parseOrgIDFromConfigKey(configKey)

	return p.processAction(action, actionKey, orgID, receivedAt)
}

// processAction processes a single action
func (p *ActionProcessor) processAction(action *kubeactions.KubeAction, actionKey ActionKey, orgID int64, receivedAt int64) error {
	actionType := GetActionType(action)
	log.Infof("[KubeActions] Received action %s (type=%s)", actionKey.String(), actionType)

	// Report that we received the action
	p.reporter.ReportReceived(actionKey, action, orgID)

	// Extract action timestamp
	var actionCreatedAt int64
	if action.Timestamp != nil {
		actionCreatedAt = action.Timestamp.GetSeconds()

		if err := ValidateTimestamp(action.Timestamp.AsTime()); err != nil {
			result := ExecutionResult{Status: StatusExpired, Message: fmt.Sprintf("timestamp validation failed: %v", err)}
			p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, actionCreatedAt)
			p.reporter.ReportResult(actionKey, action, result, orgID, time.Now())
			log.Errorf("[KubeActions] Action %s rejected: %s", actionKey.String(), result.Message)
			return err
		}
	} else {
		result := ExecutionResult{Status: StatusFailed, Message: "timestamp is missing"}
		p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, 0)
		p.reporter.ReportResult(actionKey, action, result, orgID, time.Now())
		log.Errorf("[KubeActions] Action %s rejected: %s", actionKey.String(), result.Message)
		return errors.New("action timestamp is missing")
	}

	// Validate the action
	if err := p.validator.ValidateAction(action); err != nil {
		result := ExecutionResult{Status: StatusFailed, Message: fmt.Sprintf("validation failed: %v", err)}
		p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, actionCreatedAt)
		p.reporter.ReportResult(actionKey, action, result, orgID, time.Now())
		log.Errorf("[KubeActions] Action %s rejected: %s", actionKey.String(), result.Message)
		return err
	}

	// Execute the action
	result := p.registry.Execute(p.ctx, action)

	// Store the result with all timestamps
	executedAt := time.Now()
	p.store.MarkExecuted(actionKey, result.Status, result.Message, executedAt.Unix(), receivedAt, actionCreatedAt)

	// Report the execution result to backend via Event Platform
	p.reporter.ReportResult(actionKey, action, result, orgID, executedAt)

	if result.Status == StatusSuccess {
		log.Infof("[KubeActions] Action %s executed: %s", actionKey.String(), result.Message)
		return nil
	}
	log.Errorf("[KubeActions] Action %s failed: %s", actionKey.String(), result.Message)
	return fmt.Errorf("action execution failed: %s", result.Message)
}

// GetStore returns the action store for inspection
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
