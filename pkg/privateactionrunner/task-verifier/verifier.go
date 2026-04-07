// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package taskverifier

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

// NewTaskVerifier returns a TaskVerifier appropriate for the current environment.
// When DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true, a no-op verifier is returned for e2e tests.
func NewTaskVerifier(keysManager KeysManager, cfg *config.Config) TaskVerifier {
	if os.Getenv(app.InternalSkipTaskVerificationEnvVar) == "true" {
		return &noOpTaskVerifier{}
	}
	return &signedEnvelopeTaskVerifier{keysManager: keysManager, config: cfg}
}

// noOpTaskVerifier passes tasks through without signature validation.
// Used only when DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true.
type noOpTaskVerifier struct{}

func (n *noOpTaskVerifier) UnwrapTask(task *types.Task) (*types.Task, error) {
	return task, nil
}

type signedEnvelopeTaskVerifier struct {
	keysManager KeysManager
	config      *config.Config
}

// UnwrapTask extracts and validates the task from its signed envelope.
func (t *signedEnvelopeTaskVerifier) UnwrapTask(task *types.Task) (*types.Task, error) {
	envelope := task.Data.Attributes.SignedEnvelope
	if envelope == nil {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errors.New("task is missing signed envelope"))
	}

	if len(envelope.Data) == 0 {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errors.New("data is missing"))
	}

	if len(envelope.Signatures) == 0 {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errors.New("signatures are missing"))
	}

	if envelope.HashType != privateactionspb.HashType_SHA256 {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("unsupported hash type %s", envelope.HashType))
	}
	hashedPayload := sha256.Sum256(envelope.Data)

	var pbTask privateactionspb.PrivateActionTask
	err := proto.Unmarshal(envelope.Data, &pbTask)
	if err != nil {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errors.New("failed to unmarshal task"))
	}

	if pbTask.ExpirationTime == nil {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, errors.New("expiration time is missing"))
	}

	if pbTask.ExpirationTime.AsTime().Before(time.Now()) {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_EXPIRED_TASK, errors.New("task is expired"))
	}

	signature, localKey := t.getCandidateSignatureWithKey(envelope)
	if localKey == nil {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND, errors.New("no matching key found"))
	}

	localKeyType := localKey.GetKeyType()
	if localKeyType.ToPbKeyType() != signature.KeyType {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR, fmt.Errorf("key type mismatch, expected %s, got %s", localKeyType, signature.KeyType))
	}

	err = localKey.Verify(hashedPayload[:], signature.Signature)
	if err != nil {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR, fmt.Errorf("signature verification failed: %w", err))
	}

	if pbTask.OrgId != t.config.OrgId {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_MISMATCHED_ORG_ID, errors.New("task orgId doesn't match the orgId of the runner"))
	}

	if pbTask.GetConnectionInfo().RunnerId != t.config.RunnerId {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_MISMATCHED_RUNNER_ID, errors.New("connection runnerId doesn't match the id of the runner"))
	}

	return mapPbTaskToStruct(&pbTask), nil
}

func (t *signedEnvelopeTaskVerifier) getCandidateSignatureWithKey(envelope *privateactionspb.RemoteConfigSignatureEnvelope) (*privateactionspb.Signature, types.DecodedKey) {
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

func mapPbTaskToStruct(task *privateactionspb.PrivateActionTask) *types.Task {
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
				Client:                task.Client,
				SecDatadogHeaderValue: task.SecDatadogHeaderValue,
				Inputs:                task.Inputs.AsMap(),
				OrgId:                 task.OrgId,
				ConnectionInfo:        task.ConnectionInfo,
			},
		},
	}
}
