package remoteconfig

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/pkg/keys"
	"github.com/theupdateframework/go-tuf/sign"
)

type testArtifacts struct {
	key            keys.Signer
	signedBaseRoot []byte
	rootData       *data.Root
	repository     *Repository
}

func newTestRootKey() keys.Signer {
	key, err := keys.GenerateEd25519Key()
	if err != nil {
		panic(err)
	}

	return key
}

// For now we'll just use the same key for all the roles. This isn't
// secure for production but we're not trying to test this aspect of TUF here.
func buildTestRoot(key keys.Signer) ([]byte, *data.Root) {
	root := data.NewRoot()
	root.Version = 1
	root.Expires = time.Now().Add(24 * time.Hour * 365 * 10)
	root.AddKey(key.PublicData())
	role := &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["root"] = role
	root.Roles["targets"] = role
	root.Roles["timestsmp"] = role
	root.Roles["snapshot"] = role

	rootSigners := []keys.Signer{key}
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
	key := newTestRootKey()
	signedBaseRoot, rootData := buildTestRoot(key)
	repository := NewRepository(signedBaseRoot)

	return testArtifacts{
		key:            key,
		signedBaseRoot: signedBaseRoot,
		rootData:       rootData,
		repository:     repository,
	}
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
