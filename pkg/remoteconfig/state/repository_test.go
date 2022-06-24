package state

import (
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
