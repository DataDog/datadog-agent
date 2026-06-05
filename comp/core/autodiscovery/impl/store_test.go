// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

func TestIDsOfChecksWithSecrets(t *testing.T) {
	testStore := newStore()

	testStore.setIDsOfChecksWithSecrets(map[checkid.ID]checkid.ID{
		"id1": "id2",
		"id3": "id4",
	})

	assert.Equal(t, checkid.ID("id2"), testStore.getIDOfCheckWithEncryptedSecrets("id1"))
	assert.Equal(t, checkid.ID("id4"), testStore.getIDOfCheckWithEncryptedSecrets("id3"))
	assert.Empty(t, testStore.getIDOfCheckWithEncryptedSecrets("non-existing"))

	testStore.deleteMappingsOfCheckIDsWithSecrets([]checkid.ID{"id1"})
	assert.Empty(t, testStore.getIDOfCheckWithEncryptedSecrets("id1"))

	testStore.deleteMappingsOfCheckIDsWithSecrets([]checkid.ID{"id3"})
	assert.Empty(t, testStore.getIDOfCheckWithEncryptedSecrets("id3"))
}
