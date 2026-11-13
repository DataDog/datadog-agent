// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMissingConfigFileError(t *testing.T) {
	err := NewMissingConfigFileError(os.ErrNotExist)

	assert.ErrorIs(t, err, ErrMissingConfigFile)
	assert.ErrorIs(t, err, ErrConfigFileNotFound)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
