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
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/go-multierror"
	"google.golang.org/protobuf/encoding/protojson"
)

// ActionStoreInterface defines the interface for action stores
type ActionStoreInterface interface {
	WasExecuted(key ActionKey) bool
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
	log.Infof("[KubeActions] Processor.Process called for config key: %s", configKey)

	// Validate metadata
	if rawConfig.Metadata.ID == "" {
		log.Errorf("[KubeActions] Skipping action with missing metadata.id")
		return errors.New("action metadata.id is missing")
	}
	if rawConfig.Metadata.Version == 0 {
		log.Errorf("[KubeActions] Skipping action %s with missing or zero metadata.version", rawConfig.Metadata.ID)
		return errors.New("action metadata.version is missing or zero")
	}

	log.Infof("[KubeActions] Metadata validated: ID=%s, Version=%d", rawConfig.Metadata.ID, rawConfig.Metadata.Version)

	// Parse the actions list from the config
	// NOTE: We use protojson instead of encoding/json because the KubeAction message
	// uses protobuf oneof fields (delete_pod, restart_deployment) which encoding/json
	// cannot properly unmarshal. protojson handles oneof fields correctly.
	log.Infof("[KubeActions] Attempting to unmarshal config data...")
	actionsList := &kubeactions.KubeActionsList{}
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true, // Ignore unknown fields for forward compatibility
	}
	err := unmarshaler.Unmarshal(rawConfig.Config, actionsList)
	if err != nil {
		log.Errorf("[KubeActions] Failed to unmarshal config: %v", err)
		return fmt.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v",
			rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	log.Infof("[KubeActions] Successfully unmarshaled config. Actions count: %d", len(actionsList.Actions))

	// Enforce exactly one action per config
	if len(actionsList.Actions) != 1 {
		return fmt.Errorf("expected exactly 1 action per config, got %d", len(actionsList.Actions))
	}

	// Create action key for tracking
	actionKey := ActionKey{
		ID:      rawConfig.Metadata.ID,
		Version: rawConfig.Metadata.Version,
	}

	// Record when we received this action
	receivedAt := time.Now().Unix()

	// Check if this action was already executed
	if p.store.WasExecuted(actionKey) {
		record, _ := p.store.GetRecord(actionKey)
		log.Infof("[KubeActions] Action %s was already executed with status: %s", actionKey.String(), record.Status)
		if record.Status == StatusFailed || record.Status == StatusExpired {
			return fmt.Errorf("action previously %s: %s", record.Status, record.Message)
		}
		return nil
	}

	log.Infof("[KubeActions] Action %s not yet executed, proceeding with processing", actionKey.String())

	// Extract org ID from the config key path
	orgID := parseOrgIDFromConfigKey(configKey)

	// Process all actions in the list
	var processingErrors error
	for i, action := range actionsList.Actions {
		log.Infof("[KubeActions] Processing action %d/%d", i+1, len(actionsList.Actions))
		if err := p.processAction(action, i, actionKey, orgID, receivedAt); err != nil {
			processingErrors = multierror.Append(processingErrors, err)
		}
	}

	if processingErrors != nil {
		log.Errorf("[KubeActions] Finished processing with errors: %v", processingErrors)
	} else {
		log.Infof("[KubeActions] Finished processing all actions successfully")
	}

	return processingErrors
}

// processAction processes a single action
func (p *ActionProcessor) processAction(action *kubeactions.KubeAction, index int, actionKey ActionKey, orgID int64, receivedAt int64) error {
	actionType := GetActionType(action)
	log.Infof("[KubeActions] === Processing action %d ===", index)
	log.Infof("[KubeActions]   ActionType: %s", actionType)
	if action.Resource != nil {
		log.Infof("[KubeActions]   Resource: %s/%s in %s", action.Resource.Kind, action.Resource.Name, action.Resource.Namespace)
	}

	// Report that we received the action
	p.reporter.ReportReceived(actionKey, action, orgID)

	// Extract action timestamp
	var actionCreatedAt int64
	if action.Timestamp != nil {
		actionCreatedAt = action.Timestamp.GetSeconds()
		log.Infof("[KubeActions]   Timestamp: %d (%s)", actionCreatedAt, action.Timestamp.AsTime().String())

		// Validate the timestamp
		log.Infof("[KubeActions] Validating timestamp...")
		if err := ValidateTimestamp(action.Timestamp.AsTime()); err != nil {
			log.Errorf("[KubeActions] Timestamp validation failed for %s: received timestamp %s: %v", actionKey.String(), action.Timestamp.AsTime().String(), err)
			result := ExecutionResult{Status: StatusExpired, Message: fmt.Sprintf("timestamp validation failed: %v", err)}
			p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, actionCreatedAt)
			p.reporter.ReportResult(actionKey, action, result, orgID, time.Now())
			return err
		}
		log.Infof("[KubeActions] Timestamp validation passed")
	} else {
		log.Errorf("[KubeActions] Action timestamp is missing")
		result := ExecutionResult{Status: StatusFailed, Message: "timestamp is missing"}
		p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, 0)
		p.reporter.ReportResult(actionKey, action, result, orgID, time.Now())
		return errors.New("action timestamp is missing")
	}

	// Validate the action
	log.Infof("[KubeActions] Validating action...")
	if err := p.validator.ValidateAction(action); err != nil {
		log.Errorf("[KubeActions] Action validation failed: %v", err)
		result := ExecutionResult{Status: StatusFailed, Message: fmt.Sprintf("validation failed: %v", err)}
		p.store.MarkExecuted(actionKey, result.Status, result.Message, time.Now().Unix(), receivedAt, actionCreatedAt)
		p.reporter.ReportResult(actionKey, action, result, orgID, time.Now())
		return err
	}
	log.Infof("[KubeActions] Action validation passed")

	// Execute the action
	log.Infof("[KubeActions] Executing action via registry...")
	result := p.registry.Execute(p.ctx, action)
	log.Infof("[KubeActions] Execution completed: status=%s, message=%s", result.Status, result.Message)

	// Store the result with all timestamps
	executedAt := time.Now()
	p.store.MarkExecuted(actionKey, result.Status, result.Message, executedAt.Unix(), receivedAt, actionCreatedAt)
	log.Infof("[KubeActions] Result stored in action store")

	// Report the execution result to backend via Event Platform
	p.reporter.ReportResult(actionKey, action, result, orgID, executedAt)

	// Log the result
	if result.Status == StatusSuccess {
		log.Infof("[KubeActions] Action executed successfully: %s", result.Message)
	} else {
		log.Errorf("[KubeActions] Action execution failed: %s", result.Message)
		return fmt.Errorf("action execution failed: %s", result.Message)
	}

	return nil
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
