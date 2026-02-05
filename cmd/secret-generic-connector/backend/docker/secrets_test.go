// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDockerSecretsBackendDefaultPath(t *testing.T) {
	tmpDir := t.TempDir()

	config := map[string]interface{}{
		"secrets_path": tmpDir,
	}

	backend, err := NewDockerSecretsBackend(config)
	assert.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Equal(t, tmpDir, backend.Config.SecretsPath)
}
