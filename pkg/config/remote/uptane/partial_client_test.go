package uptane

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/data"
)

func (r testRepositories) toPartialUpdate() *pbgo.ClientGetConfigsResponse {
	return &pbgo.ClientGetConfigsResponse{
		Roots:       []*pbgo.TopMeta{{Version: uint64(r.directorRootVersion), Raw: r.directorRoot}},
		Targets:     &pbgo.TopMeta{Version: uint64(r.directorTargetsVersion), Raw: r.directorTargets},
		TargetFiles: r.targetFiles,
	}
}

func TestPartialClientVerifyValid(t *testing.T) {
	target1content, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}

	testRepository := newTestRepository(1, nil, targets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	config.Datadog.Set("remote_configuration.director_root", testRepository.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", testRepository.configRoot)

	client, err := NewPartialClient()
	assert.NoError(t, err)
	err = client.Update(testRepository.toPartialUpdate())
	assert.NoError(t, err)
	targets, err := client.Targets()
	assert.NoError(t, err)
	assert.Equal(t, targets1, targets)
	targetFile, err := client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.NoError(t, err)
	assert.Equal(t, target1content, targetFile)
}

func TestPartialClientVerifyValidMissingTargetFile(t *testing.T) {
	_, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}

	testRepository := newTestRepository(1, nil, targets1, []*pbgo.File{})
	config.Datadog.Set("remote_configuration.director_root", testRepository.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", testRepository.configRoot)

	client, err := NewPartialClient()
	assert.NoError(t, err)
	err = client.Update(testRepository.toPartialUpdate())
	assert.NoError(t, err)
	targets, err := client.Targets()
	assert.NoError(t, err)
	assert.Equal(t, targets1, targets)
	_, err = client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.Error(t, err)
}

func TestPartialClientVerifyValidRotation(t *testing.T) {
	target1content, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}

	testRepository1 := newTestRepository(1, nil, targets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	config.Datadog.Set("remote_configuration.director_root", testRepository1.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", testRepository1.configRoot)
	client, err := NewPartialClient()
	assert.NoError(t, err)

	err = client.Update(testRepository1.toPartialUpdate())
	assert.NoError(t, err)

	testRepository2 := newTestRepository(2, nil, targets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	testRepository2.directorRootVersion = testRepository1.directorRootVersion + 1
	testRepository2.directorRoot = generateRoot(testRepository1.directorRootKey, testRepository2.directorRootVersion, testRepository2.directorTimestampKey, testRepository2.directorTargetsKey, testRepository2.directorSnapshotKey, nil)

	err = client.Update(testRepository2.toPartialUpdate())
	assert.NoError(t, err)
	targets, err := client.Targets()
	assert.NoError(t, err)
	assert.Equal(t, targets1, targets)
	targetFile, err := client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.NoError(t, err)
	assert.Equal(t, target1content, targetFile)
}

func TestPartialClientVerifyInvalidTargetFile(t *testing.T) {
	_, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}

	testRepository := newTestRepository(1, nil, targets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: []byte(`fakecontent`)}})
	config.Datadog.Set("remote_configuration.director_root", testRepository.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", testRepository.configRoot)

	client, err := NewPartialClient()
	assert.NoError(t, err)
	err = client.Update(testRepository.toPartialUpdate())
	assert.Error(t, err)
	_, err = client.Targets()
	assert.Error(t, err)
	_, err = client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.Error(t, err)
}

func TestPartialClientVerifyInvalidTargets(t *testing.T) {
	target1content, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}

	testRepository := newTestRepository(1, nil, targets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	config.Datadog.Set("remote_configuration.director_root", testRepository.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", testRepository.configRoot)

	testRepository.directorTargets = generateTargets(generateKey(), testRepository.directorTargetsVersion, targets1)

	client, err := NewPartialClient()
	assert.NoError(t, err)
	err = client.Update(testRepository.toPartialUpdate())
	assert.Error(t, err)
	_, err = client.Targets()
	assert.Error(t, err)
	_, err = client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.Error(t, err)
}

func TestPartialClientVerifyInvalidRotation(t *testing.T) {
	target1content, target1 := generateTarget()
	targets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}

	testRepository1 := newTestRepository(1, nil, targets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	config.Datadog.Set("remote_configuration.director_root", testRepository1.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", testRepository1.configRoot)
	client, err := NewPartialClient()
	assert.NoError(t, err)

	err = client.Update(testRepository1.toPartialUpdate())
	assert.NoError(t, err)

	testRepository2 := newTestRepository(2, nil, targets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})

	err = client.Update(testRepository2.toPartialUpdate())
	// TODO in the whole file: use "assert.ErrorAs" with specific error type
	assert.Error(t, err)
	_, err = client.Targets()
	assert.Error(t, err)
	_, err = client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.Error(t, err)
}

func TestPartialClientRootKeyRotation(t *testing.T) {
	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": targetFileMeta,
	}

	repository1 := newTestRepository(1, nil, directorTargetMetadata, nil)
	config.Datadog.Set("remote_configuration.director_root", repository1.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", repository1.configRoot)

	client, err := NewPartialClient()
	require.NoError(t, err)

	err = client.Update(repository1.toPartialUpdate())
	require.NoError(t, err)

	repository2 := newTestRepository(2, nil, directorTargetMetadata, nil)
	repository2.directorRootVersion = repository1.directorRootVersion + 1
	repository2.directorRoot = generateRoot(
		repository2.directorRootKey,
		repository2.directorRootVersion,
		repository2.directorTimestampKey,
		repository2.directorTargetsKey,
		repository2.directorSnapshotKey,
		// new root must be signed by old root
		repository1.directorRootKey,
	)

	err = client.Update(repository2.toPartialUpdate())
	require.NoError(t, err)

	root, err := client.getRoot()
	require.NoError(t, err)
	assert.Equal(t, root.Roles["root"].KeyIDs[0], repository2.directorRootKey.PublicData().IDs()[0])
}

// TestPartialClientRejectsUnsignedTarget tests that the partial uptane client
// does not accept targets which are not listed in the targets metadata file
func TestPartialClientRejectsUnsignedTarget(t *testing.T) {
	files := []*pbgo.File{
		{Path: "datadog/2/APM_SAMPLING/id/1", Raw: []byte("mAlIcIoUs cOnTeNt!!")},
	}
	// malicious target has simply be added without a signature
	directorTargetMetadata := data.TargetFiles{}

	repository := newTestRepository(1, nil, directorTargetMetadata, files)
	config.Datadog.Set("remote_configuration.director_root", repository.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", repository.configRoot)

	client, err := NewPartialClient()
	require.NoError(t, err)

	err = client.Update(repository.toPartialUpdate())
	errInvalid := &ErrInvalid{}
	require.ErrorAs(t, err, &errInvalid)
}

// TestPartialClientRejectsInvalidSignature tests that the partial uptane client
// rejects target metadata with an invalid signature
func TestPartialClientRejectsInvalidSignature(t *testing.T) {
	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": targetFileMeta,
	}

	repository := newTestRepository(1, nil, directorTargetMetadata, nil)
	config.Datadog.Set("remote_configuration.director_root", repository.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", repository.configRoot)

	// changing the signature to make it invalid
	repository.directorTargets = regexp.MustCompile(`"sig":"[a-f0-9]{6}`).
		ReplaceAll(repository.directorTargets, []byte(`"sig":"abcdef`))

	client, err := NewPartialClient()
	require.NoError(t, err)

	err = client.Update(repository.toPartialUpdate())
	errInvalid := &ErrInvalid{}
	require.ErrorAs(t, err, &errInvalid)
}

func TestPartialClientRejectsRevokedTargetsKey(t *testing.T) {
	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": targetFileMeta,
	}

	repository1 := newTestRepository(1, nil, directorTargetMetadata, nil)
	config.Datadog.Set("remote_configuration.director_root", repository1.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", repository1.configRoot)

	client, err := NewPartialClient()
	require.NoError(t, err)

	err = client.Update(repository1.toPartialUpdate())
	require.NoError(t, err)

	repository2 := newTestRepository(2, nil, directorTargetMetadata, nil)
	repository2.directorRootVersion = repository1.directorRootVersion + 1
	repository2.directorRoot = generateRoot(
		repository1.directorRootKey,
		repository2.directorRootVersion,
		repository2.directorTimestampKey,
		repository2.directorTargetsKey,
		repository2.directorSnapshotKey,
		nil,
	)

	// revoked top-level targets metadata
	repository2.directorTargets = repository1.directorTargets

	err = client.Update(repository2.toPartialUpdate())
	errInvalid := &ErrInvalid{}
	require.ErrorAs(t, err, &errInvalid)
}

func TestPartialClientRejectsRevokedRootKey(t *testing.T) {
	_, targetFileMeta := generateTarget()
	directorTargetMetadata := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": targetFileMeta,
	}

	repository1 := newTestRepository(1, nil, directorTargetMetadata, nil)
	config.Datadog.Set("remote_configuration.director_root", repository1.directorRoot)
	config.Datadog.Set("remote_configuration.config_root", repository1.configRoot)

	client, err := NewPartialClient()
	require.NoError(t, err)

	err = client.Update(repository1.toPartialUpdate())
	require.NoError(t, err)

	repository2 := newTestRepository(2, nil, directorTargetMetadata, nil)
	repository2.directorRootVersion = repository1.directorRootVersion + 1
	repository2.directorRoot = generateRoot(
		repository2.directorRootKey,
		repository2.directorRootVersion,
		repository2.directorTimestampKey,
		repository2.directorTargetsKey,
		repository2.directorSnapshotKey,
		// new root must be signed by old root
		repository1.directorRootKey,
	)

	err = client.Update(repository2.toPartialUpdate())
	require.NoError(t, err)

	// "root.json" from repository1 is only signed by root key version 1,
	// which should be now revoked
	err = client.Update(repository1.toPartialUpdate())
	errInvalid := &ErrInvalid{}
	require.ErrorAs(t, err, &errInvalid)
}
