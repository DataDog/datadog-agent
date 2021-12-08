package uptane

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
)

func TestTargetStore(t *testing.T) {
	db := getTestDB()
	store, err := newTargetStore(db, "testcachekey")
	assert.NoError(t, err)

	target1 := &pbgo.File{
		Path: "2/APM_SAMPLING/target1",
		Raw:  []byte("target1content"),
	}
	target2 := &pbgo.File{
		Path: "2/APM_SAMPLING/target2",
		Raw:  []byte("target2content"),
	}
	target3 := &pbgo.File{
		Path: "2/APM_SAMPLING/target3",
		Raw:  []byte("target3content"),
	}

	_, err = store.getTargetFile(target1.Path)
	assert.Error(t, err)

	err = store.storeTargetFiles([]*pbgo.File{target1, target2, target3})
	assert.NoError(t, err)

	returnedTarget1, err := store.getTargetFile(target1.Path)
	assert.NoError(t, err)
	assert.Equal(t, target1.Raw, returnedTarget1)
	returnedTarget2, err := store.getTargetFile(target2.Path)
	assert.NoError(t, err)
	assert.Equal(t, target2.Raw, returnedTarget2)
	returnedTarget3, err := store.getTargetFile(target3.Path)
	assert.NoError(t, err)
	assert.Equal(t, target3.Raw, returnedTarget3)

	err = store.pruneTargetFiles([]string{target1.Path, target3.Path})
	assert.NoError(t, err)
	returnedTarget1, err = store.getTargetFile(target1.Path)
	assert.NoError(t, err)
	assert.Equal(t, target1.Raw, returnedTarget1)
	_, err = store.getTargetFile(target2.Path)
	assert.Error(t, err)
	returnedTarget3, err = store.getTargetFile(target3.Path)
	assert.NoError(t, err)
	assert.Equal(t, target3.Raw, returnedTarget3)
}
