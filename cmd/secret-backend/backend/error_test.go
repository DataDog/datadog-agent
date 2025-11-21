// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package backend

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorBackend(t *testing.T) {
	backend := NewErrorBackend(fmt.Errorf("test error"))

	output := backend.GetSecretOutput("test")
	assert.NotNil(t, output.Error)
	assert.Equal(t, "test error", *output.Error)
	assert.Nil(t, output.Value)
}
