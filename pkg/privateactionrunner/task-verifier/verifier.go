package taskverifier

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"google.golang.org/protobuf/proto"
)

type TaskVerifier struct {
	keysManager remoteconfig.KeysManager
	config      *config.Config
}

func NewTaskVerifier(keysManager remoteconfig.KeysManager, config *config.Config) *TaskVerifier {
	return &TaskVerifier{keysManager: keysManager, config: config}
}

func (t *TaskVerifier) UnwrapTaskFromSignedEnvelope(envelope *privateactions.RemoteConfigSignatureEnvelope) (*types.Task, error) {
	if envelope == nil {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("task is missing signed enveloppe"))
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

	if task.OrgId != t.config.OrgId {
		return nil, utils.NewPARError(errorcode.ActionPlatformErrorCode_MISMATCHED_ORG_ID, fmt.Errorf("task orgId doesn't match the orgId of the runner"))
	}

	if task.GetConnectionInfo().RunnerId != t.config.RunnerId {
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
				OrgId:                 task.OrgId,
				ConnectionInfo:        task.ConnectionInfo,
			},
		},
	}
}
