// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package checkmetadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	// empty cache, empty payload
	p := *GetPayload()
	assert.Len(t, p, 0)

	checkID := "postgres:12345"
	name := "version.postgresql"
	value1 := "5.0.0"
	value2 := "7.0.0"

	// add a few payloads to the cache
	SetCheckMetadata(checkID, name, value1)
	SetCheckMetadata(checkID, name, value2)

	p = *GetPayload()
	assert.Len(t, p, 1)
	assert.Equal(t, value2, p[0][1])

	// GetPayload is supposed to empty the cache
	assert.Len(t, checkMetadataCache, 0)
}
