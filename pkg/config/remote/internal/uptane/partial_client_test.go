package uptane

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
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
	testRepository2.directorRoot = generateRoot(testRepository1.directorRootKey, testRepository2.directorRootVersion, testRepository2.directorTimestampKey, testRepository2.directorTargetsKey, testRepository2.directorSnapshotKey)

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
	assert.Error(t, err)
	_, err = client.Targets()
	assert.Error(t, err)
	_, err = client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.Error(t, err)
}
