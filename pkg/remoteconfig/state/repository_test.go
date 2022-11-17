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

func TestNewUnverifiedRepository(t *testing.T) {
	repository, err := NewUnverifiedRepository()
	assert.NotNil(t, repository, "Creating an unverified repository should always succeed")
	assert.Nil(t, err, "Creating an unverified repository should always succeed with no error")
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

	state, err := r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)

	// Do the same with the unverified repository, there should be no functional difference EXCEPT
	// since we don't have to start with the open source root we'll only report version 0 of the root.
	// In practice an agent will send up the root files on the first update, but this tests that if for some
	// reason it's not sent we don't do anything odd.
	r = ta.unverifiedRepository

	updatedProducts, err = r.Update(emptyUpdate)
	assert.NotNil(t, err)
	assert.Equal(t, 0, len(updatedProducts), "An empty update shouldn't indicate any updated products")
	assert.Equal(t, 0, len(r.APMConfigs()), "An empty update shoudldn't add any APM configs")
	assert.Equal(t, 0, len(r.CWSDDConfigs()), "An empty update shouldn't add any CWSDD configs")

	state, err = r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
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

	r := ta.repository

	updatedProducts, err := r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok := r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err := r.CurrentState()
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

	// Do the same with the unverified repository, there should be no functional difference
	r = ta.unverifiedRepository

	updatedProducts, err = r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok = r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err = r.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(state.Configs))
	assert.Equal(t, 1, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	configState = state.Configs[0]
	assert.Equal(t, ProductCWSDD, configState.Product)
	assert.Equal(t, "test", configState.ID)
	assert.EqualValues(t, 1, configState.Version)
	cached = state.CachedFiles[0]
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
	_, err = ta.unverifiedRepository.Update(update)
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

	r := ta.repository
	updatedProducts, err := r.Update(removalUpdate)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts))
	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 0, len(r.CWSDDConfigs()))

	state, err := r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 2, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)

	// Do the same with the unverified repository, it should be functionally identical
	r = ta.unverifiedRepository
	updatedProducts, err = r.Update(removalUpdate)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts))
	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 0, len(r.CWSDDConfigs()))

	state, err = r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 2, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
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
	_, err = ta.unverifiedRepository.Update(update)
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

	r := ta.repository
	updatedProducts, err := r.Update(update)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok := r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 2, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err := r.CurrentState()
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

	// Do the same with the unverified repository, it should be functionally identical
	r = ta.unverifiedRepository
	updatedProducts, err = r.Update(update)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok = r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 2, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err = r.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(state.Configs))
	assert.Equal(t, 1, len(state.CachedFiles))
	assert.EqualValues(t, 2, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	configState = state.Configs[0]
	assert.Equal(t, ProductCWSDD, configState.Product)
	assert.Equal(t, "test", configState.ID)
	assert.EqualValues(t, 2, configState.Version)
	cached = state.CachedFiles[0]
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

	// The unverified repository shouldn't reject this on account of the targets signature because
	// it shouldn't even look at that.
	updatedProducts, err = ta.unverifiedRepository.Update(update)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	state, err = ta.unverifiedRepository.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(state.Configs))
	assert.Equal(t, 1, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
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
	r := ta.repository
	updatedProducts, err := r.Update(update)
	assert.Error(t, err)
	assert.Equal(t, 0, len(updatedProducts))

	state, err := r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)

	// The unverified repository should still reject this update because it'll fail to parse the
	// targets file as its structurally invalid.
	r = ta.unverifiedRepository
	updatedProducts, err = r.Update(update)
	assert.Error(t, err)
	assert.Equal(t, 0, len(updatedProducts))

	state, err = r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
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

	// The unverified repository should still reject this update because it'll fail to parse the
	// root file as its structurally invalid and won't be able to extract the version
	r := ta.unverifiedRepository
	updatedProducts, err = r.Update(update)
	assert.Error(t, err)
	assert.Equal(t, 0, len(updatedProducts))

	state, err = r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
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
	r := ta.repository
	updatedProducts, err := r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok := r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err := r.CurrentState()
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

	// Do the same with the unverified repository, it should behave the same and store the latest root
	// version
	r = ta.unverifiedRepository
	updatedProducts, err = r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)

	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok = r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	state, err = r.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(state.Configs))
	assert.Equal(t, 1, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 2, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	configState = state.Configs[0]
	assert.Equal(t, ProductCWSDD, configState.Product)
	assert.Equal(t, "test", configState.ID)
	assert.EqualValues(t, 1, configState.Version)
	cached = state.CachedFiles[0]
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

	r := ta.repository
	updatedProducts, err := r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts))
	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 0, len(r.CWSDDConfigs()))

	state, err := r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)

	// Do the same with the unverified repository, it should be functionally identical
	r = ta.unverifiedRepository
	updatedProducts, err = r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts))
	assert.Equal(t, 0, len(r.APMConfigs()))
	assert.Equal(t, 0, len(r.CWSDDConfigs()))

	state, err = r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
}

func TestUpdateWithTwoProducts(t *testing.T) {
	ta := newTestArtifacts()

	file := newCWSDDFile()
	fileAPM := newAPMSamplingFile()

	path, hashes, data := addCWSDDFile("test", 1, file, ta.targets)
	pathAPM, hashesAPM := addAPMSamplingFile("testAPM", 3, fileAPM, ta.targets)
	b := signTargets(ta.key, ta.targets)

	update := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    b,
		TargetFiles:   map[string][]byte{path: data, pathAPM: fileAPM},
		ClientConfigs: []string{path, pathAPM},
	}
	r := ta.repository
	updatedProducts, err := r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)
	assert.Contains(t, updatedProducts, ProductAPMSampling)

	assert.Equal(t, 1, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok := r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	storedFileAPM, ok := r.APMConfigs()[pathAPM]
	assert.True(t, ok)
	assert.Equal(t, fileAPM, storedFileAPM.Config)
	assert.EqualValues(t, 3, storedFileAPM.Metadata.Version)
	assert.Equal(t, "testAPM", storedFileAPM.Metadata.ID)
	assertHashesEqual(t, hashesAPM, storedFileAPM.Metadata.Hashes)
	assert.Equal(t, ProductAPMSampling, storedFileAPM.Metadata.Product)
	assert.EqualValues(t, len(fileAPM), storedFileAPM.Metadata.RawLength)

	// Do the same for the unverified repository, it should be functionally identical
	r = ta.unverifiedRepository
	updatedProducts, err = r.Update(update)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(updatedProducts))
	assert.Contains(t, updatedProducts, ProductCWSDD)
	assert.Contains(t, updatedProducts, ProductAPMSampling)

	assert.Equal(t, 1, len(r.APMConfigs()))
	assert.Equal(t, 1, len(r.CWSDDConfigs()))

	storedFile, ok = r.CWSDDConfigs()[path]
	assert.True(t, ok)
	assert.Equal(t, file, storedFile.Config)
	assert.EqualValues(t, 1, storedFile.Metadata.Version)
	assert.Equal(t, "test", storedFile.Metadata.ID)
	assertHashesEqual(t, hashes, storedFile.Metadata.Hashes)
	assert.Equal(t, ProductCWSDD, storedFile.Metadata.Product)
	assert.EqualValues(t, len(data), storedFile.Metadata.RawLength)

	storedFileAPM, ok = r.APMConfigs()[pathAPM]
	assert.True(t, ok)
	assert.Equal(t, fileAPM, storedFileAPM.Config)
	assert.EqualValues(t, 3, storedFileAPM.Metadata.Version)
	assert.Equal(t, "testAPM", storedFileAPM.Metadata.ID)
	assertHashesEqual(t, hashesAPM, storedFileAPM.Metadata.Hashes)
	assert.Equal(t, ProductAPMSampling, storedFileAPM.Metadata.Product)
	assert.EqualValues(t, len(fileAPM), storedFileAPM.Metadata.RawLength)

	// Check the config state and the cached files of both repositories //

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
	expectedCachedFileCWSDD := CachedFile{
		Path:   path,
		Length: uint64(len(data)),
		Hashes: convertGoTufHashes(hashes),
	}
	expectedCachedFileAPMSampling := CachedFile{
		Path:   pathAPM,
		Length: uint64(len(fileAPM)),
		Hashes: convertGoTufHashes(hashesAPM),
	}

	r = ta.repository
	state, err := r.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(state.Configs))
	assert.Equal(t, 2, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	assert.Contains(t, state.Configs, expectedConfigStateCWSDD)
	assert.Contains(t, state.Configs, expectedConfigStateAPM)
	assert.Contains(t, state.CachedFiles, expectedCachedFileCWSDD)
	assert.Contains(t, state.CachedFiles, expectedCachedFileAPMSampling)

	r = ta.unverifiedRepository
	state, err = r.CurrentState()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(state.Configs))
	assert.Equal(t, 2, len(state.CachedFiles))
	assert.EqualValues(t, 1, state.TargetsVersion)
	assert.EqualValues(t, 0, state.RootsVersion) // 0 because we don't start with the open source root
	assert.Equal(t, testOpaqueBackendStateContents, state.OpaqueBackendState)
	assert.Contains(t, state.Configs, expectedConfigStateCWSDD)
	assert.Contains(t, state.Configs, expectedConfigStateAPM)
	assert.Contains(t, state.CachedFiles, expectedCachedFileCWSDD)
	assert.Contains(t, state.CachedFiles, expectedCachedFileAPMSampling)
}

// These tests involve generated JSON responses that the remote config service would send. They
// were primarily created to assist tracer teams with unit tests around TUF integrity checks,
// but they apply to agent clients as well, so we include them here as an extra layer of protection.
//
// These should only be done against the verified version of the repository.
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
		{description: "valid", isError: false, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTItMjlUMDg6NTg6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvQVNNX0ZFQVRVUkVTL0FTTV9GRUFUVVJFUy1iYXNlL2NvbmZpZyI6eyJjdXN0b20iOnsidiI6MX0sImhhc2hlcyI6eyJzaGEyNTYiOiI5MjIxZGZkOWY2MDg0MTUxMzEzZTNlNDkyMDEyMWFlODQzNjE0YzMyOGU0NjMwZWEzNzFiYTY2ZTJmMTVhMGE2In0sImxlbmd0aCI6NDd9fSwidmVyc2lvbiI6MX0sInNpZ25hdHVyZXMiOlt7ImtleWlkIjoiZWQ3NjcyYzlhMjRhYmRhNzg4NzJlZTMyZWU3MWM3Y2IxZDUyMzVlOGRiNGVjYmYxY2EyOGI5YzUwZWI3NWQ5ZSIsInNpZyI6IjlhNWYyOTAzNTkzNTdiZmJkOTk3NzJkYzFiYTAzYzc3N2E2NjhkZjA4YzRiYmQ4MDdkMGI5ZGI2ZDUyYzNhOTgyZmY3ODA0MTY0MGUxZWMwZDU1ZDQxNDBkMGE3YzViOGRhYjgwM2NlM2ZjMDk1MzFlNWJjZWJlMTkwMTZiNjA0In1dfQ==","target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`)},
		{description: "invalid tuf targets signature", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidmVyc2lvbiI6OTk5LCJleHBpcmVzIjoiMjAyMi0xMC0yN1QxNjo1Mzo1N1oiLCJ0YXJnZXRzIjp7ImRhdGFkb2cvMi9GRUFUVVJFUy9GRUFUVVJFUy1iYXNlL2NvbmZpZyI6eyJsZW5ndGgiOjQ3LCJoYXNoZXMiOnsic2hhMjU2IjoiOTIyMWRmZDlmNjA4NDE1MTMxM2UzZTQ5MjAxMjFhZTg0MzYxNGMzMjhlNDYzMGVhMzcxYmE2NmUyZjE1YTBhNiJ9LCJjdXN0b20iOnsidiI6MX19fSwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifX0sInNpZ25hdHVyZXMiOlt7ImtleWlkIjoiZWQ3NjcyYzlhMjRhYmRhNzg4NzJlZTMyZWU3MWM3Y2IxZDUyMzVlOGRiNGVjYmYxY2EyOGI5YzUwZWI3NWQ5ZSIsInNpZyI6ImQ3NDhiNjM5ODk1M2I4YjA5Zjc5NzMwYjZiOTc5NmQ1YzY3ZjIyYjY3Y2QxNzgyMmY2MjYyOThmNTUyNDllYzk0YmQ4YTYyY2IyMjBhMjRiZGJhMmJjZGFlM2NhMGQ0ZDhkOGE5MWNjZDY1NzFjYTYxNzUwOWQ0MmI1MzI2NjAzIn1dfQ==","target_files":[{"path":"datadog/2/ASM_FEATURES/asm_features_activation/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/asm_features_activation/config"]}`)},
		{description: "tuf targets signed with invalid key", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTItMjlUMDg6NTg6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvQVNNX0ZFQVRVUkVTL0FTTV9GRUFUVVJFUy1iYXNlL2NvbmZpZyI6eyJjdXN0b20iOnsidiI6MX0sImhhc2hlcyI6eyJzaGEyNTYiOiI5MjIxZGZkOWY2MDg0MTUxMzEzZTNlNDkyMDEyMWFlODQzNjE0YzMyOGU0NjMwZWEzNzFiYTY2ZTJmMTVhMGE2In0sImxlbmd0aCI6NDd9fSwidmVyc2lvbiI6MX0sInNpZ25hdHVyZXMiOlt7ImtleWlkIjoiOTM5NjFhZWU2Njk2MWQyMzJlMDAwOGEzYmIxMTY4NzliYzVjMzM3NWFlOTliNWZiOTliYTA5MGM0ZDJmYTIzYiIsInNpZyI6IjA5ZWZjN2MxNGUxMzc0M2Y1MWE1ZWI1ODZjYTFlMzY3NWVmZmFiNjk2NjlmYzVkNTY0MGI4Y2I3NGI0NWQ0MTY4ODZhODkwYzNkOGI0ODFhZGM1Y2Y5ZGZkZTYxZDlhMzUxYzcxNGY2MzUzY2ViZTFmZWZhOTNmMjhmNDI5OTBhIn1dfQ==","target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`)},
		{description: "missing target file in tuf targets", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTItMjlUMDg6NTg6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6e30sInZlcnNpb24iOjF9LCJzaWduYXR1cmVzIjpbeyJrZXlpZCI6ImVkNzY3MmM5YTI0YWJkYTc4ODcyZWUzMmVlNzFjN2NiMWQ1MjM1ZThkYjRlY2JmMWNhMjhiOWM1MGViNzVkOWUiLCJzaWciOiI4N2U3N2JjNTJlZjNhNWM4OGZlNGY5MTVlYTgxNjVjNTM4ODJhYTUzZDcyMjRiYWZiZTA2ZmFhMDYxYzc5MDA5YjkzZjEyYTk4NTM3ZmY3ZGVhMGZhYTI2ZGUxZjdjOWYzN2Q0YjAzZTFmY2U0ZjFjMDQxZjBmMzBjOGNmYmUwZSJ9XX0=","target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`)},
		{description: "target file hash incorrect in tuf targets", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTItMjlUMDg6NTg6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvQVNNX0ZFQVRVUkVTL0FTTV9GRUFUVVJFUy1iYXNlL2NvbmZpZyI6eyJjdXN0b20iOnsidiI6MX0sImhhc2hlcyI6eyJzaGEyNTYiOiI2NjYxNmI2NTY4NjE3MzY4In0sImxlbmd0aCI6NDd9fSwidmVyc2lvbiI6MX0sInNpZ25hdHVyZXMiOlt7ImtleWlkIjoiZWQ3NjcyYzlhMjRhYmRhNzg4NzJlZTMyZWU3MWM3Y2IxZDUyMzVlOGRiNGVjYmYxY2EyOGI5YzUwZWI3NWQ5ZSIsInNpZyI6IjkzYzQ3MWMxMWM3NzM1ZDViZmI4OTJhYmNlY2I5MGYyYzFhZTJlNDlmODNlYzdiMmNmOGYxZmZiZTY4MmEwM2E5OGI0MTA2NWRlOWVjZmM1MzZkYzU5Njk2NjM4NjUxYWEyYmNlOWNiYzViMzI3YmUwYzM2ZGRhOWZhZTBhMjAzIn1dfQ==","target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`)},
		{description: "target file length incorrect in tuf targets", isError: true, rawUpdate: []byte(`{"targets":"eyJzaWduZWQiOnsiX3R5cGUiOiJ0YXJnZXRzIiwiY3VzdG9tIjp7Im9wYXF1ZV9iYWNrZW5kX3N0YXRlIjoiZXlKbWIyOGlPaUFpWW1GeUluMD0ifSwiZXhwaXJlcyI6IjIwMjItMTItMjlUMDg6NTg6MzJaIiwic3BlY192ZXJzaW9uIjoiMS4wIiwidGFyZ2V0cyI6eyJkYXRhZG9nLzIvQVNNX0ZFQVRVUkVTL0FTTV9GRUFUVVJFUy1iYXNlL2NvbmZpZyI6eyJjdXN0b20iOnsidiI6MX0sImhhc2hlcyI6eyJzaGEyNTYiOiI2NjYxNmI2NTY4NjE3MzY4In0sImxlbmd0aCI6OTk5fX0sInZlcnNpb24iOjF9LCJzaWduYXR1cmVzIjpbeyJrZXlpZCI6ImVkNzY3MmM5YTI0YWJkYTc4ODcyZWUzMmVlNzFjN2NiMWQ1MjM1ZThkYjRlY2JmMWNhMjhiOWM1MGViNzVkOWUiLCJzaWciOiJiZDJiMTY3ZmQ4N2UxM2VhYjdhZjdlOWIwZDYwYWZkYjBiYjAzY2U1Y2QyOTc4OWUzMDgxNTY0NjViYTg1NzA0MDU0ZWJlMTAzODdhYWJiZjM3MGI1ODJjODJlMDc4MjhkMWI1NTYyODEzYzkzMTJhY2NhMmJlNWVkNzA2ZjUwNiJ9XX0=","target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`)},
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
