// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package taskverifier provides functionality for verifying and unwrapping tasks from signed envelopes.
package taskverifier

import (
	"crypto/sha256"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

// TaskVerifier verifies and unwraps tasks from signed envelopes.
type TaskVerifier struct {
	keysManager remoteconfig.KeysManager
	config      *config.Config
}

// NewTaskVerifier creates a new TaskVerifier instance.
func NewTaskVerifier(keysManager remoteconfig.KeysManager, config *config.Config) *TaskVerifier {
	return &TaskVerifier{keysManager: keysManager, config: config}
}

// UnwrapTaskFromSignedEnvelope verifies and unwraps a task from a signed envelope.
func (t *TaskVerifier) UnwrapTaskFromSignedEnvelope(envelope *privateactions.RemoteConfigSignatureEnvelope) (*types.Task, error) {
	if envelope == nil {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("task is missing signed envelope"))
	}

	if len(envelope.Data) == 0 {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("data is missing"))
	}

	if len(envelope.Signatures) == 0 {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("signatures are missing"))
	}

	if envelope.HashType != privateactions.HashType_SHA256 {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("unsupported hash type %s", envelope.HashType))
	}
	hashedPayload := sha256.Sum256(envelope.Data)

	var task privateactions.PrivateActionTask
	err := proto.Unmarshal(envelope.Data, &task)
	if err != nil {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("failed to unmarshal task"))
	}

	if task.ExpirationTime == nil {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("expiration time is missing"))
	}

	if task.ExpirationTime.AsTime().Before(time.Now()) {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_EXPIRED_TASK, fmt.Errorf("task is expired"))
	}

	signature, localKey := t.getCandidateSignatureWithKey(envelope)
	if localKey == nil {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND, fmt.Errorf("no matching key found"))
	}

	localKeyType := localKey.GetKeyType()
	if localKeyType.ToPbKeyType() != signature.KeyType {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_SIGNATURE_ERROR, fmt.Errorf("key type mismatch, expected %s, got %s", localKeyType, signature.KeyType))
	}

	err = localKey.Verify(hashedPayload[:], signature.Signature)
	if err != nil {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_SIGNATURE_ERROR, fmt.Errorf("signature verification failed: %w", err))
	}

	if task.OrgId != t.config.OrgID {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_MISMATCHED_ORG_ID, fmt.Errorf("task orgId doesn't match the orgId of the runner"))
	}

	if task.GetConnectionInfo().RunnerId != t.config.RunnerID {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_MISMATCHED_RUNNER_ID, fmt.Errorf("connection runnerId doesn't match the id of the runner"))
	}

	return mapPbTaskToStruct(&task), nil
}

func (t *TaskVerifier) getCandidateSignatureWithKey(envelope *privateactions.RemoteConfigSignatureEnvelope) (*privateactions.Signature, types.DecodedKey) {
	if len(envelope.Signatures) == 0 {
		return nil, nil
	}
	for _, sig := range envelope.Signatures {
		localKey := t.keysManager.GetKey(sig.KeyId)
		if localKey != nil {
			return sig, localKey
		}
	}

	return nil, nil
}

func mapPbTaskToStruct(task *privateactions.PrivateActionTask) *types.Task {
	return &types.Task{
		Data: struct {
			ID         string            `json:"id,omitempty"`
			Type       string            `json:"type,omitempty"`
			Attributes *types.Attributes `json:"attributes,omitempty"`
		}{
			ID:   task.TaskId,
			Type: "task",
			Attributes: &types.Attributes{
				Name:                  task.ActionName,
				BundleID:              task.BundleId,
				SecDatadogHeaderValue: task.SecDatadogHeaderValue,
				Inputs:                task.Inputs.AsMap(),
				OrgID:                 task.OrgId,
				ConnectionInfo:        task.ConnectionInfo,
			},
		},
	}
}
