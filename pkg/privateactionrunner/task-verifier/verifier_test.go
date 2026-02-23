// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package taskverifier

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticKeysManager is a test double that returns keys from an in-memory map.
type staticKeysManager struct {
	keys map[string]types.DecodedKey
}

func (s *staticKeysManager) Start(_ context.Context)          {}
func (s *staticKeysManager) WaitForReady()                    {}
func (s *staticKeysManager) GetKey(id string) types.DecodedKey { return s.keys[id] }

// testConfig returns a minimal config with a known OrgId and RunnerId.
func testConfig(orgID int64, runnerID string) *config.Config {
	return &config.Config{OrgId: orgID, RunnerId: runnerID}
}

// buildValidTask returns a PrivateActionTask protobuf for the given orgID and runnerID
// with an expiration one hour in the future.
func buildValidTask(t *testing.T, orgID int64, runnerID string) *privateactionspb.PrivateActionTask {
	t.Helper()
	inputs, err := structpb.NewStruct(map[string]interface{}{})
	require.NoError(t, err)
	return &privateactionspb.PrivateActionTask{
		TaskId:         "task-001",
		ActionName:     "doSomething",
		BundleId:       "com.example.bundle",
		OrgId:          orgID,
		ExpirationTime: timestamppb.New(time.Now().Add(time.Hour)),
		Inputs:         inputs,
		ConnectionInfo: &privateactionspb.ConnectionInfo{RunnerId: runnerID},
	}
}

// signTask marshals task, computes SHA-256, and signs with privKey to create a valid envelope.
func signTask(t *testing.T, task *privateactionspb.PrivateActionTask, privKey ed25519.PrivateKey, keyID string) *privateactionspb.RemoteConfigSignatureEnvelope {
	t.Helper()
	data, err := proto.Marshal(task)
	require.NoError(t, err)
	hash := sha256.Sum256(data)
	sig := ed25519.Sign(privKey, hash[:])
	return &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     data,
		HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{
			{KeyId: keyID, KeyType: privateactionspb.KeyType_ED25519, Signature: sig},
		},
	}
}

// TestUnwrapTask_NilEnvelope verifies that a nil envelope is rejected with INTERNAL_ERROR.
func TestUnwrapTask_NilEnvelope(t *testing.T) {
	v := NewTaskVerifier(&staticKeysManager{}, testConfig(1, "r1"))

	_, err := v.UnwrapTaskFromSignedEnvelope(nil)

	require.Error(t, err)
	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, nil)
}

// TestUnwrapTask_EmptyData verifies that an envelope with no data is rejected.
func TestUnwrapTask_EmptyData(t *testing.T) {
	v := NewTaskVerifier(&staticKeysManager{}, testConfig(1, "r1"))
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     []byte{},
		HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{
			{KeyId: "k1", KeyType: privateactionspb.KeyType_ED25519, Signature: []byte("sig")},
		},
	}

	_, err := v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, nil)
}

// TestUnwrapTask_MissingSignatures verifies that an envelope with data but no signatures is rejected.
func TestUnwrapTask_MissingSignatures(t *testing.T) {
	v := NewTaskVerifier(&staticKeysManager{}, testConfig(1, "r1"))
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:       []byte("some data"),
		HashType:   privateactionspb.HashType_SHA256,
		Signatures: nil,
	}

	_, err := v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, nil)
}

// TestUnwrapTask_UnsupportedHashType verifies that only SHA256 is accepted as the hash algorithm.
func TestUnwrapTask_UnsupportedHashType(t *testing.T) {
	v := NewTaskVerifier(&staticKeysManager{}, testConfig(1, "r1"))
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     []byte("data"),
		HashType: privateactionspb.HashType_HASH_TYPE_UNKNOWN,
		Signatures: []*privateactionspb.Signature{
			{KeyId: "k1", Signature: []byte("sig")},
		},
	}

	_, err := v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, nil)
}

// TestUnwrapTask_InvalidProtobufData verifies that garbage bytes in the envelope data
// cause an INTERNAL_ERROR (unmarshal failure).
func TestUnwrapTask_InvalidProtobufData(t *testing.T) {
	v := NewTaskVerifier(&staticKeysManager{}, testConfig(1, "r1"))
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     []byte("not valid protobuf"),
		HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{
			{KeyId: "k1", Signature: []byte("sig")},
		},
	}

	_, err := v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, nil)
}

// TestUnwrapTask_MissingExpiration verifies that a task without an expiration time is rejected.
func TestUnwrapTask_MissingExpiration(t *testing.T) {
	v := NewTaskVerifier(&staticKeysManager{}, testConfig(1, "r1"))
	task := &privateactionspb.PrivateActionTask{TaskId: "t1"}
	data, _ := proto.Marshal(task)
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     data,
		HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{
			{KeyId: "k1", Signature: []byte("sig")},
		},
	}

	_, err := v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, nil)
}

// TestUnwrapTask_ExpiredTask verifies that a task whose expiration is in the past is rejected
// with the dedicated EXPIRED_TASK error code.
func TestUnwrapTask_ExpiredTask(t *testing.T) {
	v := NewTaskVerifier(&staticKeysManager{}, testConfig(1, "r1"))
	task := &privateactionspb.PrivateActionTask{
		TaskId:         "t1",
		ExpirationTime: timestamppb.New(time.Now().Add(-time.Hour)),
	}
	data, _ := proto.Marshal(task)
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     data,
		HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{
			{KeyId: "k1", Signature: []byte("sig")},
		},
	}

	_, err := v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_EXPIRED_TASK, nil)
}

// TestUnwrapTask_NoMatchingKey verifies that when the key ID in the signature is not found
// in the KeysManager, the verifier returns SIGNATURE_KEY_NOT_FOUND.
func TestUnwrapTask_NoMatchingKey(t *testing.T) {
	// KeysManager is empty – no keys registered.
	v := NewTaskVerifier(&staticKeysManager{keys: map[string]types.DecodedKey{}}, testConfig(1, "r1"))
	task := &privateactionspb.PrivateActionTask{
		TaskId:         "t1",
		ExpirationTime: timestamppb.New(time.Now().Add(time.Hour)),
	}
	data, _ := proto.Marshal(task)
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     data,
		HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{
			{KeyId: "unknown-key-id", KeyType: privateactionspb.KeyType_ED25519, Signature: []byte("sig")},
		},
	}

	_, err := v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND, nil)
}

// TestUnwrapTask_InvalidSignature verifies that a signature that does not verify against the
// registered public key returns SIGNATURE_ERROR.
func TestUnwrapTask_InvalidSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keys := map[string]types.DecodedKey{
		"key1": types.ED25519Key{KeyType: types.KeyTypeED25519, Key: pub},
	}
	v := NewTaskVerifier(&staticKeysManager{keys: keys}, testConfig(1, "r1"))

	task := buildValidTask(t, 1, "r1")
	data, _ := proto.Marshal(task)
	env := &privateactionspb.RemoteConfigSignatureEnvelope{
		Data:     data,
		HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{
			{KeyId: "key1", KeyType: privateactionspb.KeyType_ED25519, Signature: []byte("invalid-sig")},
		},
	}

	_, err = v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR, nil)
}

// TestUnwrapTask_OrgIDMismatch verifies that a valid task signed with the correct key but
// belonging to a different org is rejected with MISMATCHED_ORG_ID.
func TestUnwrapTask_OrgIDMismatch(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keys := map[string]types.DecodedKey{
		"key1": types.ED25519Key{KeyType: types.KeyTypeED25519, Key: pub},
	}
	// Config says orgID=1 but the task carries orgID=999.
	v := NewTaskVerifier(&staticKeysManager{keys: keys}, testConfig(1, "r1"))
	task := buildValidTask(t, 999, "r1")
	env := signTask(t, task, priv, "key1")

	_, err = v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_MISMATCHED_ORG_ID, nil)
}

// TestUnwrapTask_RunnerIDMismatch verifies that a valid task signed with the correct key but
// addressed to a different runner is rejected with MISMATCHED_RUNNER_ID.
func TestUnwrapTask_RunnerIDMismatch(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keys := map[string]types.DecodedKey{
		"key1": types.ED25519Key{KeyType: types.KeyTypeED25519, Key: pub},
	}
	// Config says runnerID="r1" but the task is addressed to "r-other".
	v := NewTaskVerifier(&staticKeysManager{keys: keys}, testConfig(1, "r1"))
	task := buildValidTask(t, 1, "r-other")
	env := signTask(t, task, priv, "key1")

	_, err = v.UnwrapTaskFromSignedEnvelope(env)

	requirePARErrorCode(t, err, aperrorpb.ActionPlatformErrorCode_MISMATCHED_RUNNER_ID, nil)
}

// TestUnwrapTask_HappyPath verifies that a fully valid signed envelope is accepted and that
// the unwrapped task preserves the task ID and action name.
func TestUnwrapTask_HappyPath(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keys := map[string]types.DecodedKey{
		"key1": types.ED25519Key{KeyType: types.KeyTypeED25519, Key: pub},
	}
	v := NewTaskVerifier(&staticKeysManager{keys: keys}, testConfig(42, "runner-xyz"))
	task := buildValidTask(t, 42, "runner-xyz")
	env := signTask(t, task, priv, "key1")

	result, err := v.UnwrapTaskFromSignedEnvelope(env)

	require.NoError(t, err)
	assert.Equal(t, "task-001", result.Data.ID)
	assert.Equal(t, "doSomething", result.Data.Attributes.Name)
	assert.Equal(t, "com.example.bundle", result.Data.Attributes.BundleID)
	assert.Equal(t, int64(42), result.Data.Attributes.OrgId)
}

// PARErrorForTest is a helper used by requirePARErrorCode to extract the error code.
type PARErrorForTest = interface{ GetErrorCode() aperrorpb.ActionPlatformErrorCode }

// requirePARErrorCode asserts that err is a PARError with the given error code. If pe is
// non-nil it is populated with the error so callers can make additional assertions.
func requirePARErrorCode(t *testing.T, err error, want aperrorpb.ActionPlatformErrorCode, pe interface{}) {
	t.Helper()
	require.Error(t, err)
	var parErr interface {
		error
		GetErrorCode() aperrorpb.ActionPlatformErrorCode
		GetMessage() string
	}

	// The returned error from UnwrapTaskFromSignedEnvelope is util.PARError which embeds
	// *aperrorpb.ActionPlatformError and implements the error interface.
	// We perform a structural check via type assertion to the embedded protobuf type.
	type hasCode interface {
		GetErrorCode() aperrorpb.ActionPlatformErrorCode
	}
	coded, ok := err.(hasCode)
	if !ok {
		// Try unwrapping (PARError implements error directly)
		type unwrapper interface{ Unwrap() error }
		if uw, ok2 := err.(unwrapper); ok2 {
			coded, ok = uw.Unwrap().(hasCode)
		}
	}
	require.True(t, ok, "expected a PARError with GetErrorCode(), got %T: %v", err, err)
	assert.Equal(t, want, coded.GetErrorCode())
	_ = parErr
}
