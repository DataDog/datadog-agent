// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRepositoryWithNilRoot(t *testing.T) {
	repository, err := NewRepository(nil)
	assert.Nil(t, repository, "Creating a repository without a starting base root should result in an error per TUF spec")
	assert.Error(t, err, "Creating a repository without a starting base root should result in an error per TUF spec")
}

func TestNewRepositoryWithMalformedRoot(t *testing.T) {
	repository, err := NewRepository([]byte("haha I am not a real root"))
	assert.Nil(t, repository, "Creating a repository with a malformed base root should result in an error per TUF spec")
	assert.Error(t, err, "Creating a repository with a malformed base root should result in an error per TUF spec")
}

func TestEmptyUpdate(t *testing.T) {
	ta := newTestArtifacts()

	emptyUpdate := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    make([]byte, 0),
		TargetFiles:   make(map[string][]byte),
		ClientConfigs: make([]string, 0),
	}

	r := ta.repository

	updatedProducts, err := r.Update(emptyUpdate)
	assert.NotNil(t, err)
	assert.Equal(t, 0, len(updatedProducts), "An empty update shouldn't indicate any updated products")
	assert.Equal(t, 0, len(r.APMConfigs()), "An empty update shoudldn't add any APM configs")
	assert.Equal(t, 0, len(r.CWSDDConfigs()), "An empty update shouldn't add any CWSDD configs")

	state, err := ta.repository.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)
}

func TestUpdateNewConfig(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	path, hashes, data := addCWSDDFile("test", 1, file, ta.targets)
	b := signTargets(ta.key, ta.targets)

	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	updatedProducts, err := ta.repository.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(ta.repository.APMConfigs()))
	assert.Equal(t, 1, len(ta.repository.CWSDDConfigs()))

	storedFile, ok := ta.repository.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err := ta.repository.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(state.Configs))
	assert.Equal(t, 1, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	configState := state.Configs[0]
	assert.Equal(t, ProductCWSDD, configState.Product)
	assert.Equal(t, "test", configState.ID)
	assert.EqualValues(t, 1, configState.Version)
	cached := state.CachedFiles[0]
	assert.Equal(t, path, cached.Path)
	assert.EqualValues(t, len(data), cached.Length)
	assertHashesEqual(t, hashes, cached.Hashes)
}

func TestUpdateNewConfigThenRemove(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	path, _, data := addCWSDDFile("test", 1, file, ta.targets)
	b := signTargets(ta.key, ta.targets)

	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	_, err := ta.repository.Update(update)
	assert.Nil(t, err)

	// We test this exact update in another test, so we'll just carry on here.
	// TODO: Make it possible to create the repository at this valid state just to
	// clean up tests if we accidentally introduce a bug that breaks this first update.

	delete(ta.targets.Targets, path)
	ta.targets.Version = 2
	b = signTargets(ta.key, ta.targets)
	removalUpdate := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   make(map[string][]byte),
		ClientConfigs: make([]string, 0),
	}
	updatedProducts, err := ta.repository.Update(removalUpdate)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts))
	assert.Equal(t, 0, len(ta.repository.APMConfigs()))
	assert.Equal(t, 0, len(ta.repository.CWSDDConfigs()))

	state, err := ta.repository.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 2, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
}

func TestUpdateNewConfigThenModify(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	path, _, data := addCWSDDFile("test", 1, file, ta.targets)
	b := signTargets(ta.key, ta.targets)
	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	_, err := ta.repository.Update(update)
	assert.Nil(t, err)

	file = []byte("updated file")
	path, hashes, data := addCWSDDFile("test", 2, file, ta.targets)
	ta.targets.Version = 2
	b = signTargets(ta.key, ta.targets)
	update = Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	updatedProducts, err := ta.repository.Update(update)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(ta.repository.APMConfigs()))
	assert.Equal(t, 1, len(ta.repository.CWSDDConfigs()))

	storedFile, ok := ta.repository.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 2, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err := ta.repository.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(state.Configs))
	assert.Equal(t, 1, len(state.CachedFiles))
	assert.EqualValues(t, 2, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	configState := state.Configs[0]
	assert.Equal(t, ProductCWSDD, configState.Product)
	assert.Equal(t, "test", configState.ID)
	assert.EqualValues(t, 2, configState.Version)
	cached := state.CachedFiles[0]
	assert.Equal(t, path, cached.Path)
	assert.EqualValues(t, len(data), cached.Length)
	assertHashesEqual(t, hashes, cached.Hashes)

}

func TestUpdateWithIncorrectlySignedTargets(t *testing.T) {
	ta := newTestArtifacts()

	fakeKey := newTestKey()

	file := newCWSDDFile()
	path, _, data := addCWSDDFile("test", 1, file, ta.targets)
	b := signTargets(fakeKey, ta.targets)
	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	updatedProducts, err := ta.repository.Update(update)
	assert.Error(t, err)
	assert.Equal(t, 0, len(updatedProducts))

	state, err := ta.repository.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)
}

func TestUpdateWithMalformedTargets(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	path, _, data := addCWSDDFile("test", 1, file, ta.targets)
	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    []byte("haha i am not a targets"),
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	updatedProducts, err := ta.repository.Update(update)
	assert.Error(t, err)
	assert.Equal(t, 0, len(updatedProducts))

	state, err := ta.repository.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)
}

func TestUpdateWithMalformedExtraRoot(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	path, _, data := addCWSDDFile("test", 1, file, ta.targets)
	b := signTargets(ta.key, ta.targets)
	update := Update{
		TUFRoots:      [][]byte{[]byte("haha I am not a root")},
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	updatedProducts, err := ta.repository.Update(update)
	assert.Error(t, err)
	assert.Equal(t, 0, len(updatedProducts))

	state, err := ta.repository.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)
}

func TestUpdateWithNewRoot(t *testing.T) {
	ta := newTestArtifacts()
	newTargetsKey := newTestKey()

	// The new root will use a new targets key to make sure that we use the
	// updated root data for validation of the update payload
	newRootRaw, _ := buildTestRoot(ta.key, newTargetsKey, 2)

	file := newCWSDDFile()
	path, hashes, data := addCWSDDFile("test", 1, file, ta.targets)
	b := signTargets(newTargetsKey, ta.targets)

	update := Update{
		TUFRoots:      [][]byte{newRootRaw},
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: []string{path},
	}
	updatedProducts, err := ta.repository.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(ta.repository.APMConfigs()))
	assert.Equal(t, 1, len(ta.repository.CWSDDConfigs()))

	storedFile, ok := ta.repository.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err := ta.repository.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(state.Configs))
	assert.Equal(t, 1, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 2, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	configState := state.Configs[0]
	assert.Equal(t, ProductCWSDD, configState.Product)
	assert.Equal(t, "test", configState.ID)
	assert.EqualValues(t, 1, configState.Version)
	cached := state.CachedFiles[0]
	assert.Equal(t, path, cached.Path)
	assert.EqualValues(t, len(data), cached.Length)
	assertHashesEqual(t, hashes, cached.Hashes)
}

func TestClientOnlyTakesActionOnFilesInClientConfig(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	path, _, data := addCWSDDFile("test", 1, file, ta.targets)
	b := signTargets(ta.key, ta.targets)

	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data},
		ClientConfigs: make([]string, 0),
	}

	updatedProducts, err := ta.repository.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts))
	assert.Equal(t, 0, len(ta.repository.APMConfigs()))
	assert.Equal(t, 0, len(ta.repository.CWSDDConfigs()))

	state, err := ta.repository.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
}

func TestUpdateWithTwoProducts(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	fileAPM := newAPMSamplingFile()

	path, hashes, data := addCWSDDFile("test", 1, file, ta.targets)
	pathAPM, hashesAPM, dataAPM := addAPMSamplingFile("testAPM", 3, fileAPM, ta.targets)
	b := signTargets(ta.key, ta.targets)

	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data, pathAPM: dataAPM},
		ClientConfigs: []string{path, pathAPM},
	}
	updatedProducts, err := ta.repository.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)
	assert.Contains(t, updatedProducts, ProductAPMSampling)

	assert.Equal(t, 1, len(ta.repository.APMConfigs()))
	assert.Equal(t, 1, len(ta.repository.CWSDDConfigs()))

	storedFile, ok := ta.repository.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	storedFileAPM, ok := ta.repository.APMConfigs()[pathAPM]
	assert.True(t, ok)
	assert.Equal(t, fileAPM, storedFileAPM.Config)
	assert.EqualValues(t, 3, storedFileAPM.Metadata.Version)
	assert.Equal(t, "testAPM", storedFileAPM.Metadata.ID)
	assertHashesEqual(t, hashesAPM, storedFileAPM.Metadata.Hashes)
	assert.Equal(t, ProductAPMSampling, storedFileAPM.Metadata.Product)
	assert.EqualValues(t, len(dataAPM), storedFileAPM.Metadata.RawLength)

	state, err := ta.repository.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(state.Configs))
	assert.Equal(t, 2, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)

	expectedConfigStateCWSDD := ConfigState{
		Product: ProductCWSDD,
		ID:      "test",
		Version: 1,
	}
	expectedConfigStateAPM := ConfigState{
		Product: ProductAPMSampling,
		ID:      "testAPM",
		Version: 3,
	}
	assert.Contains(t, state.Configs, expectedConfigStateCWSDD)
	assert.Contains(t, state.Configs, expectedConfigStateAPM)

	expectedCachedFileCWSDD := CachedFile{
		Path:   path,
		Length: uint64(len(data)),
		Hashes: convertGoTufHashes(hashes),
	}
	expectedCachedFileAPMSampling := CachedFile{
		Path:   pathAPM,
		Length: uint64(len(dataAPM)),
		Hashes: convertGoTufHashes(hashesAPM),
	}
	assert.Contains(t, state.CachedFiles, expectedCachedFileCWSDD)
	assert.Contains(t, state.CachedFiles, expectedCachedFileAPMSampling)
}

// These tests involve generated JSON responses that the remote config service would send. They
// were primarily created to assist tracer teams with unit tests around TUF integrity checks,
// but they apply to agent clients as well, so we include them here as an extra layer of protection.
func TestPreGeneratedIntegrityChecks(t *testing.T) {
	testRoot := []byte(`{"signed":{"_type":"root","spec_version":"1.0","version":1,"expires":"2032-05-29T12:49:41.030418-04:00","keys":{"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e":{"keytype":"ed25519","scheme":"ed25519","keyid_hash_algorithms":["sha256","sha512"],"keyval":{"public":"7d3102e39abe71044d207550bda239c71380d013ec5a115f79f51622630054e6"}}},"roles":{"root":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1},"snapshot":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1},"targets":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1},"timestsmp":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1}},"consistent_snapshot":true},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"d7e24828d1d3104e48911860a13dd6ad3f4f96d45a9ea28c4a0f04dbd3ca6c205ed406523c6c4cacfb7ebba68f7e122e42746d1c1a83ffa89c8bccb6f7af5e06"}]}`)

	type testData struct {
		description string
		isError     bool
		rawUpdate   []byte
	}

	type pregeneratedResponse struct {
		Targets     []byte `json:"targets"`
		TargetFiles []struct {
			Path string `json:"path"`
			Raw  []byte `json:"raw"`
		} `json:"target_files"`
		ClientConfigs []string `json:"client_configs"`
	}

	tests := []testData{
		{description: "valid", isError: false, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTEtMDNUMTg6MDE6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvRkVBVFVSRVMvRkVBVFVSRVMtYmFzZS9jb25maWciOnsiY3VzdG9tIjp7InYiOjF9LCJoYXNoZXMiOnsic2hhMjU2IjoiOTIyMWRmZDlmNjA4NDE1MTMxM2UzZTQ5MjAxMjFhZTg0MzYxNGMzMjhlNDYzMGVhMzcxYmE2NmUyZjE1YTBhNiJ9LCJsZW5ndGgiOjQ3fX0sInZlcnNpb24iOjF9LCJzaWduYXR1cmVzIjpbeyJrZXlpZCI6ImVkNzY3MmM5YTI0YWJkYTc4ODcyZWUzMmVlNzFjN2NiMWQ1MjM1ZThkYjRlY2JmMWNhMjhiOWM1MGViNzVkOWUiLCJzaWciOiI1ZGZhMjc5ZjRhY2U3NTgwODY0NDRiODYxNWI1ZDg1MDQ3YmM0ZDE2ZWFkZjUwZDU2YTFjNjMyM2ViM2ZiNjU0MzgwOTljZGQzZjI2Njg0NjQ4OTBjZDAzYWU0ODA2ZTk5MjZhN2FhZTI1OTkzOWU4NjQ1ZGE2ODYwN2Y1YTQwMSJ9XX0=","target_files":[{"path":"datadog/2/FEATURES/FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/FEATURES/FEATURES-base/config"]}`)},
		{description: "invalid tuf targets signature", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidmVyc2lvbiI6OTk5LCJleHBpcmVzIjoiMjAyMi0xMS0wM1QxODowMTozMloiLCJ0YXJnZXRzIjp7ImRhdGFkb2cvMi9GRUFUVVJFUy9GRUFUVVJFUy1iYXNlL2NvbmZpZyI6eyJsZW5ndGgiOjQ3LCJoYXNoZXMiOnsic2hhMjU2IjoiOTIyMWRmZDlmNjA4NDE1MTMxM2UzZTQ5MjAxMjFhZTg0MzYxNGMzMjhlNDYzMGVhMzcxYmE2NmUyZjE1YTBhNiJ9LCJjdXN0b20iOnsidiI6MX19fSwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifX0sInNpZ25hdHVyZXMiOlt7ImtleWlkIjoiZWQ3NjcyYzlhMjRhYmRhNzg4NzJlZTMyZWU3MWM3Y2IxZDUyMzVlOGRiNGVjYmYxY2EyOGI5YzUwZWI3NWQ5ZSIsInNpZyI6IjVkZmEyNzlmNGFjZTc1ODA4NjQ0NGI4NjE1YjVkODUwNDdiYzRkMTZlYWRmNTBkNTZhMWM2MzIzZWIzZmI2NTQzODA5OWNkZDNmMjY2ODQ2NDg5MGNkMDNhZTQ4MDZlOTkyNmE3YWFlMjU5OTM5ZTg2NDVkYTY4NjA3ZjVhNDAxIn1dfQ==","target_files":[{"path":"datadog/2/FEATURES/FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/FEATURES/FEATURES-base/config"]}`)},
		{description: "tuf targets signed with invalid key", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTEtMDNUMTg6MDE6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvRkVBVFVSRVMvRkVBVFVSRVMtYmFzZS9jb25maWciOnsiY3VzdG9tIjp7InYiOjF9LCJoYXNoZXMiOnsic2hhMjU2IjoiOTIyMWRmZDlmNjA4NDE1MTMxM2UzZTQ5MjAxMjFhZTg0MzYxNGMzMjhlNDYzMGVhMzcxYmE2NmUyZjE1YTBhNiJ9LCJsZW5ndGgiOjQ3fX0sInZlcnNpb24iOjF9LCJzaWduYXR1cmVzIjpbeyJrZXlpZCI6ImJjZjljMWFiYTk5ZTkzZDEyN2VjYjQ4ZDMwZjkwYWVjMzY3ZDRjMWEyNzY4Nzg4NTkwNzA4ZjVkOWM5MGY0ODMiLCJzaWciOiJkM2QzN2MxNDNlMGQ2Y2EzZTRiYzQ1N2U3YjFhYTU1MDA5NzFmOTZmYWFkNjBjNTk1ZjM1NDIxZGJmOWIyMzgyMThmZjViMjkwMTBiMjM4MmU2Yjg3ZmQxOTBjYTM4MDVjYzY3NDBlNzdiMzQxYjlmZDI1YjYzOGQ0MDcwZWYwNyJ9XX0=","target_files":[{"path":"datadog/2/FEATURES/FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/FEATURES/FEATURES-base/config"]}`)},
		{description: "missing target file in tuf targets", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTEtMDNUMTg6MDE6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6e30sInZlcnNpb24iOjF9LCJzaWduYXR1cmVzIjpbeyJrZXlpZCI6ImVkNzY3MmM5YTI0YWJkYTc4ODcyZWUzMmVlNzFjN2NiMWQ1MjM1ZThkYjRlY2JmMWNhMjhiOWM1MGViNzVkOWUiLCJzaWciOiI4YTIyNjE0MDFkMzI4NTk0ZjlkZGQ4NDZmYzc2ZjM2MjZmYmJhMjczZTgxNTRlNjVhMTM0NGZiNDI0OGM3ZTE1YmFmYjM0NjY0NmM0ZmY3OTdhZGIyMjE2Nzg4NDQwYjQ5NGZlYThmYzAwZWVkMGY5MzZlNzRlNDM5NDEyYzIwOCJ9XX0=","target_files":[{"path":"datadog/2/FEATURES/FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/FEATURES/FEATURES-base/config"]}`)},
		{description: "target file hash incorrect in tuf targets", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTEtMDNUMTg6MDE6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvRkVBVFVSRVMvRkVBVFVSRVMtYmFzZS9jb25maWciOnsiY3VzdG9tIjp7InYiOjF9LCJoYXNoZXMiOnsic2hhMjU2IjoiNjY2MTZiNjU2ODYxNzM2OCJ9LCJsZW5ndGgiOjQ3fX0sInZlcnNpb24iOjF9LCJzaWduYXR1cmVzIjpbeyJrZXlpZCI6ImVkNzY3MmM5YTI0YWJkYTc4ODcyZWUzMmVlNzFjN2NiMWQ1MjM1ZThkYjRlY2JmMWNhMjhiOWM1MGViNzVkOWUiLCJzaWciOiI3N2E5ODdmMTAyOGE3ZTdhOTU3YmY0Zjc1MjVjNzJlYmQwZTQzYWI2YTY1MmRjYTY2MGVlMjAzNmViMzgwYzVlNzQ4MTk2ZTVjYzcwZDc1NGVkMzQwNzQ4NTcwZTEzN2I5MzY3NmQwNTNjYmY0OTA4NTFlNTIyNGZkMzcxYzgwNyJ9XX0=","target_files":[{"path":"datadog/2/FEATURES/FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/FEATURES/FEATURES-base/config"]}`)},
		{description: "target file length incorrect in tuf targets", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTEtMDNUMTg6MDE6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvRkVBVFVSRVMvRkVBVFVSRVMtYmFzZS9jb25maWciOnsiY3VzdG9tIjp7InYiOjF9LCJoYXNoZXMiOnsic2hhMjU2IjoiNjY2MTZiNjU2ODYxNzM2OCJ9LCJsZW5ndGgiOjk5OX19LCJ2ZXJzaW9uIjoxfSwic2lnbmF0dXJlcyI6W3sia2V5aWQiOiJlZDc2NzJjOWEyNGFiZGE3ODg3MmVlMzJlZTcxYzdjYjFkNTIzNWU4ZGI0ZWNiZjFjYTI4YjljNTBlYjc1ZDllIiwic2lnIjoiN2U4YmE2NGQ5YzVlY2Q5YWU3MDEzY2JmNTNlNDg1YzFjYjQzZDI1OGJhMzA3OTk3NWFiYTE3OTI0NGE4YTgxYzU4NTRjM2JhMTliOTRiMzkyODVjOWJjZDdjN2UwYWE3NWM3OWFjN2Y3MWQ2MjIzMzRhYjA3ZDhiNDMxZDIyMGEifV19","target_files":[{"path":"datadog/2/FEATURES/FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/FEATURES/FEATURES-base/config"]}`)},
	}

	for _, test := range tests {
		// These payloads are the ClientGetConfigsResponse JSON from the protobuf layer, so we have
		// to do a little processing first to be able to use them here in this internal package that is protobuf layer agnostic.
		var parsed pregeneratedResponse
		err := json.Unmarshal(test.rawUpdate, &parsed)
		assert.NoError(t, err)
		updateFiles := make(map[string][]byte)
		for _, f := range parsed.TargetFiles {
			updateFiles[f.Path] = f.Raw
		}
		update := Update{
			TUFTargets:    parsed.Targets,
			TargetFiles:   updateFiles,
			ClientConfigs: parsed.ClientConfigs,
		}

		repository, err := NewRepository(testRoot)
		assert.NoError(t, err)

		result, err := repository.Update(update)
		if test.isError {
			assert.Error(t, err, test.description)
			assert.Nil(t, result)
		} else {
			assert.NoError(t, err)
			assert.NotNil(t, result)
		}
	}
}
