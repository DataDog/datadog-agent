// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultEntityIDFromStr(t *testing.T) {
	str := "container_id://1234"
	entityID, err := NewEntityIDFromString(str)
	require.NoError(t, err)
	assert.Equal(t, ContainerID, entityID.GetPrefix())
	assert.Equal(t, "1234", entityID.GetID())
}
