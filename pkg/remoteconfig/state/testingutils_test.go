package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/stretchr/testify/assert"
	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/pkg/keys"
	"github.com/theupdateframework/go-tuf/sign"
	"github.com/theupdateframework/go-tuf/util"
)

type testArtifacts struct {
	key            keys.Signer
	signedBaseRoot []byte
	root           *data.Root
	targets        *data.Targets
	repository     *Repository
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

	targets := data.NewTargets()
	targets.Version = 1

	return testArtifacts{
		key:            key,
		signedBaseRoot: signedBaseRoot,
		root:           root,
		targets:        targets,
		repository:     repository,
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

func newAPMSamplingFile() apmsampling.APMSampling {
	apmConfig := apmsampling.APMSampling{
		TargetTPS: []apmsampling.TargetTPS{{
			Service:   "test service",
			Env:       "test env",
			Value:     0.5,
			Rank:      0,
			Mechanism: apmsampling.SamplingMechanism(6),
		}},
	}

	return apmConfig
}

func addAPMSamplingFile(id string, version int64, file apmsampling.APMSampling, targets *data.Targets) (string, data.Hashes, []byte) {
	path := fmt.Sprintf("datadog/3/%s/%s/config", ProductAPMSampling, id)

	buf := make([]byte, 0, file.Msgsize())
	out, err := file.MarshalMsg(buf)
	if err != nil {
		panic(err)
	}

	tfm := generateRCTargetFileMeta(out, version)

	targets.Targets[path] = tfm

	return path, tfm.Hashes, out
}

func convertGoTufHashes(hashes data.Hashes) map[string][]byte {
	converted := make(map[string][]byte)

	for algo, hash := range hashes {
		converted[algo] = hash
	}

	return converted
}
