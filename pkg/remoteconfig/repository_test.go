package remoteconfig

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
}
