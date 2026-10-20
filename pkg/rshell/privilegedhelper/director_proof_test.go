// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelper

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/go-tuf/data"
	"github.com/DataDog/go-tuf/pkg/keys"
	"github.com/DataDog/go-tuf/sign"
	"github.com/DataDog/go-tuf/util"
	"github.com/stretchr/testify/require"
)

func directorMaterial(t *testing.T, orgID int64, keyID string, taskPublic ed25519.PublicKey) (json.RawMessage, CredentialKey) {
	t.Helper()
	directorKey, err := keys.GenerateEd25519Key()
	require.NoError(t, err)

	root := data.NewRoot()
	root.Version = 1
	root.Expires = time.Now().Add(10 * 365 * 24 * time.Hour)
	root.AddKey(directorKey.PublicData())
	role := &data.Role{KeyIDs: directorKey.PublicData().IDs(), Threshold: 1}
	root.Roles["root"] = role
	root.Roles["targets"] = role
	root.Roles["snapshot"] = role
	root.Roles["timestamp"] = role
	signedRoot, err := sign.Marshal(&root, directorKey)
	require.NoError(t, err)
	rootJSON, err := json.Marshal(signedRoot)
	require.NoError(t, err)

	publicDER, err := x509.MarshalPKIXPublicKey(taskPublic)
	require.NoError(t, err)
	targetFile, err := json.Marshal(remoteConfigKey{
		KeyType: KeyTypeED25519,
		Key:     pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}),
	})
	require.NoError(t, err)
	targetPath := fmt.Sprintf("datadog/%d/%s/%s/config", orgID, state.ProductActionPlatformRunnerKeys, keyID)
	targetMeta, err := util.GenerateTargetFileMeta(bytes.NewReader(targetFile), "sha256", "sha512")
	require.NoError(t, err)
	custom := json.RawMessage(`{"v":1}`)
	targetMeta.Custom = &custom
	targets := data.NewTargets()
	targets.Version = 1
	targets.Expires = time.Now().Add(time.Hour)
	targets.Targets[targetPath] = targetMeta
	signedTargets, err := sign.Marshal(targets, directorKey)
	require.NoError(t, err)
	targetsJSON, err := json.Marshal(signedTargets)
	require.NoError(t, err)

	proofJSON, err := json.Marshal(DirectorKeyProof{
		Targets:    targetsJSON,
		TargetPath: targetPath,
		TargetFile: targetFile,
	})
	require.NoError(t, err)
	return rootJSON, CredentialKey{ID: targetPath, Type: KeyTypeTUFDirector, PEM: string(proofJSON)}
}

func TestDirectorProofAuthenticatesSocketTaskKey(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	root, material := directorMaterial(t, 42, "key-1", public)
	credential, _ := testCredential(t)
	credential.DirectorRoot = root
	credential.decodedKeys = map[string]verificationKey{}

	requestCredential, err := credential.withSocketVerificationKeys([]CredentialKey{material})
	require.NoError(t, err)
	verified, err := requestCredential.Verify(signedRequest(t, private, nil), time.Now())
	require.NoError(t, err)
	require.Equal(t, "task-1", verified.TaskID)

	_, err = credential.Verify(signedRequest(t, private, nil), time.Now())
	require.EqualError(t, err, "no trusted signature found")
}

func TestDirectorProofRejectsTamperedTarget(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	root, material := directorMaterial(t, 42, "key-1", public)
	credential, _ := testCredential(t)
	credential.DirectorRoot = root
	credential.decodedKeys = map[string]verificationKey{}

	var proof DirectorKeyProof
	require.NoError(t, json.Unmarshal([]byte(material.PEM), &proof))
	proof.TargetFile[0] ^= 0xff
	tampered, err := json.Marshal(proof)
	require.NoError(t, err)
	material.PEM = string(tampered)

	_, err = credential.withSocketVerificationKeys([]CredentialKey{material})
	require.ErrorContains(t, err, "verify Director proof")
}

func TestDirectorProofRejectsTargetsFromUntrustedSigner(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	trustedRoot, _ := directorMaterial(t, 42, "key-1", public)
	_, untrustedMaterial := directorMaterial(t, 42, "key-1", public)
	credential, _ := testCredential(t)
	credential.DirectorRoot = trustedRoot
	credential.decodedKeys = map[string]verificationKey{}

	_, err = credential.withSocketVerificationKeys([]CredentialKey{untrustedMaterial})
	require.ErrorContains(t, err, "verify Director proof")
}

func TestDirectorProofRejectsWrongOrg(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	root, material := directorMaterial(t, 43, "key-1", public)
	credential, _ := testCredential(t)
	credential.DirectorRoot = root
	credential.decodedKeys = map[string]verificationKey{}

	_, err = credential.withSocketVerificationKeys([]CredentialKey{material})
	require.EqualError(t, err, "AP_RUNNER_KEYS target org does not match helper credential")
}

func TestPolicyWithoutDirectorRootAcceptsBareVerificationKey(t *testing.T) {
	credential, private := testCredential(t)
	credential.decodedKeys = map[string]verificationKey{}

	requestCredential, err := credential.withSocketVerificationKeys([]CredentialKey{socketCredentialKey(t, private)})
	require.NoError(t, err)
	require.Len(t, requestCredential.decodedKeys, 1)
	require.False(t, requestCredential.trustBackendPolicy)
}
