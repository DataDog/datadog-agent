// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/go-tuf/data"
	"github.com/DataDog/go-tuf/pkg/keys"
	"github.com/DataDog/go-tuf/sign"
	"github.com/DataDog/go-tuf/util"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
)

var (
	testOpaqueBackendStateContents = []byte(`{"foo": "bar"}`)
)

type testArtifacts struct {
	key                  keys.Signer
	signedBaseRoot       []byte
	root                 *data.Root
	targets              *data.Targets
	repository           *Repository
	unverifiedRepository *Repository
}

func newTestKey() keys.Signer {
	key, err := keys.GenerateEd25519Key()
	if err != nil {
		panic(err)
	}

	return key
}

// For now we'll just use root for timestamp and snapshot, since we're not actually validating this
// in tracer clients that will use the `Repository`. We'll allow for a distinct targets role to make
// testing easier
func buildTestRoot(rootKey keys.Signer, targetsKey keys.Signer, version int64) ([]byte, *data.Root) {
	root := data.NewRoot()
	root.Version = version
	root.Expires = time.Now().Add(24 * time.Hour * 365 * 10)
	root.AddKey(rootKey.PublicData())
	root.AddKey(targetsKey.PublicData())
	rootRole := &data.Role{
		KeyIDs:    rootKey.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["root"] = rootRole
	targetsRole := &data.Role{
		KeyIDs:    targetsKey.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["targets"] = targetsRole
	root.Roles["timestamp"] = rootRole
	root.Roles["snapshot"] = rootRole

	rootSigners := []keys.Signer{rootKey}
	signedRoot, err := sign.Marshal(&root, rootSigners...)
	if err != nil {
		panic(err)
	}
	signedRootBytes, err := json.Marshal(&signedRoot)
	if err != nil {
		panic(err)
	}

	return signedRootBytes, root
}

func newTestArtifacts() testArtifacts {
	key := newTestKey()
	signedBaseRoot, root := buildTestRoot(key, key, 1)
	repository, err := NewRepository(signedBaseRoot)
	if err != nil {
		panic(err)
	}

	unverifiedRepository, err := NewUnverifiedRepository()
	if err != nil {
		panic(err)
	}

	state := struct {
		State []byte `json:"opaque_backend_state"`
	}{[]byte(testOpaqueBackendStateContents)}

	b, err := json.Marshal(&state)
	if err != nil {
		panic(err)
	}
	rm := json.RawMessage(b)

	targets := data.NewTargets()
	targets.Version = 1
	targets.Custom = &rm

	return testArtifacts{
		key:                  key,
		signedBaseRoot:       signedBaseRoot,
		root:                 root,
		targets:              targets,
		repository:           repository,
		unverifiedRepository: unverifiedRepository,
	}
}

func signTargets(key keys.Signer, targets *data.Targets) []byte {
	signed, err := sign.Marshal(targets, key)
	if err != nil {
		panic(err)
	}

	b, err := json.Marshal(signed)
	if err != nil {
		panic(err)
	}

	return b
}

func assertHashesEqual(t *testing.T, expectedHashes data.Hashes, hashes map[string][]byte) {
	assert.Equal(t, len(expectedHashes), len(hashes))

	for algorithm, hash := range expectedHashes {
		storedHash, ok := hashes[algorithm]
		assert.True(t, ok)
		assert.EqualValues(t, storedHash, hash)
	}
}

func generateFileMetaCustom(version int64) json.RawMessage {
	v := struct {
		Version int64 `json:"v"`
	}{version}

	b, err := json.Marshal(&v)
	if err != nil {
		panic(err)
	}

	return json.RawMessage(b)
}

func generateRCTargetFileMeta(data []byte, version int64) data.TargetFileMeta {
	tfm, err := util.GenerateTargetFileMeta(bytes.NewBuffer(data), []string{"sha256", "sha512"}...)
	if err != nil {
		panic(err)
	}
	custom := generateFileMetaCustom(version)
	tfm.FileMeta.Custom = &custom

	return tfm
}

func newCWSDDFile() []byte {
	data := []byte("cwsddfile")
	return data
}

func addCWSDDFile(id string, version int64, file []byte, targets *data.Targets) (string, data.Hashes, []byte) {
	path := fmt.Sprintf("datadog/3/%s/%s/config", ProductCWSDD, id)
	tfm := generateRCTargetFileMeta(file, version)

	targets.Targets[path] = tfm

	return path, tfm.Hashes, file
}

func newAPMSamplingFile() []byte {
	tps := float64(42)
	enabled := true
	samplerConfig := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			PrioritySamplerTargetTPS: &tps,
			ErrorsSamplerTargetTPS:   &tps,
			RareSamplerEnabled:       &enabled,
		},
		ByEnv: []apmsampling.EnvAndConfig{
			{Env: "some-env", Config: apmsampling.SamplerEnvConfig{
				PrioritySamplerTargetTPS: &tps,
				ErrorsSamplerTargetTPS:   &tps,
				RareSamplerEnabled:       &enabled,
			}},
		},
	}

	raw, _ := json.Marshal(samplerConfig)

	return raw
}

func addAPMSamplingFile(id string, version int64, file []byte, targets *data.Targets) (string, data.Hashes) {
	path := fmt.Sprintf("datadog/3/%s/%s/config", ProductAPMSampling, id)

	tfm := generateRCTargetFileMeta(file, version)

	targets.Targets[path] = tfm

	return path, tfm.Hashes
}

func convertGoTufHashes(hashes data.Hashes) map[string][]byte {
	converted := make(map[string][]byte)

	for algo, hash := range hashes {
		converted[algo] = hash
	}

	return converted
}
