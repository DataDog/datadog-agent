// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package uptane

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/data"
)

func TestPartialClientVerifyValid(t *testing.T) {
	target1content, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": target1,
	}

	testRepository := newTestRepository(1, nil, targets1, map[string][]byte{"datadog/2/APM_SAMPLING/id/config": target1content})

	client := NewPartialClient(testRepository.directorRoot)
	targets, err := client.Update(nil, &PartialClientTargets{}, testRepository.directorTargets, testRepository.targetFiles)
	assert.NoError(t, err)
	assert.Equal(t, targets1, targets.Targets())
	targetFile, found := targets.TargetFile("datadog/2/APM_SAMPLING/id/config")
	assert.True(t, found)
	assert.Equal(t, target1content, targetFile)
}

func TestPartialClientVerifyValidMissingTargetFile(t *testing.T) {
	_, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": target1,
	}
	testRepository := newTestRepository(1, nil, targets1, map[string][]byte{})

	client := NewPartialClient(testRepository.directorRoot)
	targets, err := client.Update(nil, &PartialClientTargets{}, testRepository.directorTargets, testRepository.targetFiles)
	assert.NoError(t, err)
	assert.Equal(t, targets1, targets.Targets())
	_, found := targets.TargetFile("datadog/2/APM_SAMPLING/id/config")
	assert.False(t, found)
}

func TestPartialClientVerifyValidRotation(t *testing.T) {
	target1content, target1 := generateTarget()
	targetsFile1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": target1,
	}

	testRepository1 := newTestRepository(1, nil, targetsFile1, map[string][]byte{"datadog/2/APM_SAMPLING/id/config": target1content})
	client := NewPartialClient(testRepository1.directorRoot)

	targets1, err := client.Update(nil, &PartialClientTargets{}, testRepository1.directorTargets, testRepository1.targetFiles)
	assert.NoError(t, err)

	testRepository2 := newTestRepository(2, nil, targetsFile1, map[string][]byte{"datadog/2/APM_SAMPLING/id/config": target1content})
	testRepository2.directorRootVersion = testRepository1.directorRootVersion + 1
	testRepository2.directorRoot = generateRoot(testRepository1.directorRootKey, testRepository2.directorRootVersion, testRepository2.directorTargetsKey, nil)

	targets2, err := client.Update([][]byte{testRepository2.directorRoot}, targets1, testRepository2.directorTargets, testRepository2.targetFiles)
	assert.NoError(t, err)
	assert.Equal(t, targetsFile1, targets2.Targets())
	targetFile, found := targets2.TargetFile("datadog/2/APM_SAMPLING/id/config")
	assert.True(t, found)
	assert.Equal(t, target1content, targetFile)
}

func TestPartialClientVerifyInvalidTargetFile(t *testing.T) {
	_, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": target1,
	}

	testRepository := newTestRepository(1, nil, targets1, map[string][]byte{"datadog/2/APM_SAMPLING/id/config": []byte(`fakecontent`)})

	client := NewPartialClient(testRepository.directorRoot)
	_, err := client.Update(nil, &PartialClientTargets{}, testRepository.directorTargets, testRepository.targetFiles)
	assert.Error(t, err)
}

func TestPartialClientVerifyInvalidTargets(t *testing.T) {
	target1content, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": target1,
	}

	testRepository := newTestRepository(1, nil, targets1, map[string][]byte{"datadog/2/APM_SAMPLING/id/config": target1content})
	testRepository.directorTargets = generateTargets(generateKey(), testRepository.directorTargetsVersion, targets1)

	client := NewPartialClient(testRepository.directorRoot)
	_, err := client.Update(nil, &PartialClientTargets{}, testRepository.directorTargets, testRepository.targetFiles)
	assert.Error(t, err)
}

func TestPartialClientVerifyInvalidRotation(t *testing.T) {
	target1content, target1 := generateTarget()
	targets1file := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": target1,
	}

	testRepository1 := newTestRepository(1, nil, targets1file, map[string][]byte{"datadog/2/APM_SAMPLING/id/config": target1content})
	client := NewPartialClient(testRepository1.directorRoot)

	targets1, err := client.Update(nil, &PartialClientTargets{}, testRepository1.directorTargets, testRepository1.targetFiles)
	assert.NoError(t, err)

	testRepository2 := newTestRepository(2, nil, targets1file, map[string][]byte{"datadog/2/APM_SAMPLING/id/config": target1content})

	_, err = client.Update([][]byte{testRepository2.directorRoot}, targets1, testRepository2.directorTargets, testRepository2.targetFiles)
	// TODO in the whole file: use "assert.ErrorAs" with specific error type
	assert.Error(t, err)
}

func TestPartialClientRootKeyRotation(t *testing.T) {
	require := require.New(t)

	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": targetFileMeta,
	}

	repository1 := newTestRepository(1, nil, directorTargetMetadata, nil)

	client := NewPartialClient(repository1.directorRoot)

	targets1, err := client.Update(nil, &PartialClientTargets{}, repository1.directorTargets, repository1.targetFiles)
	require.NoError(err)

	repository2 := newTestRepository(2, nil, directorTargetMetadata, nil)
	repository2.directorRootVersion = repository1.directorRootVersion + 1
	repository2.directorRoot = generateRoot(
		repository2.directorRootKey,
		repository2.directorRootVersion,
		repository2.directorTargetsKey,
		// new root must be signed by old root
		repository1.directorRootKey,
	)

	_, err = client.Update([][]byte{repository2.directorRoot}, targets1, repository2.directorTargets, repository2.targetFiles)
	require.NoError(err)

	root, err := client.getRoot()
	require.NoError(err)
	assert.Equal(t, root.Roles["root"].KeyIDs[0], repository2.directorRootKey.PublicData().IDs()[0])
}

// TestPartialClientRejectsUnsignedTarget tests that the partial uptane client
// does not accept targets which are not listed in the targets metadata file
func TestPartialClientRejectsUnsignedTarget(t *testing.T) {
	require := require.New(t)

	files := map[string][]byte{
		"datadog/2/APM_SAMPLING/id/config": []byte("mAlIcIoUs cOnTeNt!!"),
	}
	// malicious target has simply be added without a signature
	directorTargetMetadata := data.TargetFiles{}

	repository := newTestRepository(1, nil, directorTargetMetadata, files)

	client := NewPartialClient(repository.directorRoot)

	targets, err := client.Update(nil, &PartialClientTargets{}, repository.directorTargets, repository.targetFiles)
	require.NoError(err)
	_, evilFileFound := targets.TargetFile("datadog/2/APM_SAMPLING/id/config")
	require.False(evilFileFound)
}

// TestPartialClientRejectsInvalidSignature tests that the partial uptane client
// rejects target metadata with an invalid signature
func TestPartialClientRejectsInvalidSignature(t *testing.T) {
	require := require.New(t)

	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": targetFileMeta,
	}

	repository := newTestRepository(1, nil, directorTargetMetadata, nil)

	// changing the signature to make it invalid
	repository.directorTargets = regexp.MustCompile(`"sig":"[a-f0-9]{6}`).
		ReplaceAll(repository.directorTargets, []byte(`"sig":"abcdef`))

	client := NewPartialClient(repository.directorRoot)

	_, err := client.Update(nil, &PartialClientTargets{}, repository.directorTargets, repository.targetFiles)
	errInvalid := &ErrInvalid{}
	require.ErrorAs(err, &errInvalid)
}

func TestPartialClientRejectsRevokedTargetsKey(t *testing.T) {
	require := require.New(t)

	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": targetFileMeta,
	}

	repository1 := newTestRepository(1, nil, directorTargetMetadata, nil)

	client := NewPartialClient(repository1.directorRoot)
	target1, err := client.Update(nil, &PartialClientTargets{}, repository1.directorTargets, repository1.targetFiles)
	require.NoError(err)

	repository2 := newTestRepository(2, nil, directorTargetMetadata, nil)
	repository2.directorRootVersion = repository1.directorRootVersion + 1
	repository2.directorRoot = generateRoot(
		repository1.directorRootKey,
		repository2.directorRootVersion,
		repository2.directorTargetsKey,
		nil,
	)

	// revoked top-level targets metadata
	repository2.directorTargets = repository1.directorTargets

	_, err = client.Update([][]byte{repository2.directorRoot}, target1, repository2.directorTargets, repository2.targetFiles)
	errInvalid := &ErrInvalid{}
	require.ErrorAs(err, &errInvalid)
}

func TestPartialClientRejectsRevokedRootKey(t *testing.T) {
	require := require.New(t)

	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/config": targetFileMeta,
	}

	repository1 := newTestRepository(1, nil, directorTargetMetadata, nil)

	client := NewPartialClient(repository1.directorRoot)
	targets1, err := client.Update(nil, &PartialClientTargets{}, repository1.directorTargets, repository1.targetFiles)
	require.NoError(err)

	repository2 := newTestRepository(2, nil, directorTargetMetadata, nil)
	repository2.directorRootVersion = repository1.directorRootVersion + 1
	repository2.directorRoot = generateRoot(
		repository2.directorRootKey,
		repository2.directorRootVersion,
		repository2.directorTargetsKey,
		// new root must be signed by old root
		repository1.directorRootKey,
	)

	targets2, err := client.Update([][]byte{repository2.directorRoot}, targets1, repository2.directorTargets, repository2.targetFiles)
	require.NoError(err)

	// "root.json" from repository1 is only signed by root key version 1,
	// which should be now revoked
	_, err = client.Update(nil, targets2, repository1.directorTargets, repository1.targetFiles)
	errInvalid := &ErrInvalid{}
	require.ErrorAs(err, &errInvalid)
}
