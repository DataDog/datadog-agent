// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package uptane

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/pkg/keys"
	"github.com/theupdateframework/go-tuf/sign"
)

func generateKey() keys.Signer {
	key, _ := keys.GenerateEd25519Key()
	return key
}

type testRepositories struct {
	directorTargetsKey keys.Signer
	directorRootKey    keys.Signer

	directorTargetsVersion int
	directorRootVersion    int

	directorTargets []byte
	directorRoot    []byte

	targetFiles map[string][]byte
}

func newTestRepository(version int, configTargets data.TargetFiles, directorTargets data.TargetFiles, targetFiles map[string][]byte) testRepositories {
	repos := testRepositories{
		directorTargetsKey: generateKey(),
		directorRootKey:    generateKey(),
		targetFiles:        targetFiles,
	}
	repos.directorRootVersion = version
	repos.directorTargetsVersion = 200 + version
	repos.directorRoot = generateRoot(repos.directorRootKey, version, repos.directorTargetsKey, nil)
	repos.directorTargets = generateTargets(repos.directorTargetsKey, 200+version, directorTargets)
	return repos
}

func generateRoot(key keys.Signer, version int, targetsKey keys.Signer, previousRootKey keys.Signer) []byte {
	root := data.NewRoot()
	root.Version = version
	root.Expires = time.Now().Add(1 * time.Hour)
	root.AddKey(key.PublicData())
	root.AddKey(targetsKey.PublicData())
	root.Roles["root"] = &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["targets"] = &data.Role{
		KeyIDs:    targetsKey.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["timestamp"] = &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["snapshot"] = &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	rootSigners := []keys.Signer{key}
	if previousRootKey != nil {
		rootSigners = append(rootSigners, previousRootKey)
	}

	signedRoot, _ := sign.Marshal(&root, rootSigners...)
	serializedRoot, _ := json.Marshal(signedRoot)
	return serializedRoot
}

func generateTargets(key keys.Signer, version int, targets data.TargetFiles) []byte {
	meta := data.NewTargets()
	meta.Expires = time.Now().Add(1 * time.Hour)
	meta.Version = version
	meta.Targets = targets
	signed, _ := sign.Marshal(&meta, key)
	serialized, _ := json.Marshal(signed)
	return serialized
}

func hashSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func generateTarget() ([]byte, data.TargetFileMeta) {
	file := make([]byte, 128)
	rand.Read(file)
	return file, data.TargetFileMeta{
		FileMeta: data.FileMeta{
			Length: int64(len(file)),
			Hashes: data.Hashes{
				"sha256": hashSha256(file),
			},
		},
	}
}
