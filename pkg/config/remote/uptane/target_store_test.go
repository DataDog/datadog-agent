// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestTargetStore(t *testing.T) {
	db := newTransactionalStore(getTestDB(t))
	defer db.commit()

	store := newTargetStore(db)

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

	_, found, err := store.getTargetFile(target1.Path)
	assert.False(t, found)
	assert.NoError(t, err)

	err = store.storeTargetFiles([]*pbgo.File{target1, target2, target3})
	assert.NoError(t, err)

	returnedTarget1, found, err := store.getTargetFile(target1.Path)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, target1.Raw, returnedTarget1)
	returnedTarget2, found, err := store.getTargetFile(target2.Path)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, target2.Raw, returnedTarget2)
	returnedTarget3, found, err := store.getTargetFile(target3.Path)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, target3.Raw, returnedTarget3)

	err = store.pruneTargetFiles([]string{target1.Path, target3.Path})
	assert.NoError(t, err)
	returnedTarget1, found, err = store.getTargetFile(target1.Path)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, target1.Raw, returnedTarget1)
	_, found, err = store.getTargetFile(target2.Path)
	assert.False(t, found)
	assert.NoError(t, err)
	returnedTarget3, found, err = store.getTargetFile(target3.Path)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, target3.Raw, returnedTarget3)
}
