// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && !windows
// +build docker,!windows

package docker

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetPath(t *testing.T) {
	t.Run("use_podman_logs=false", func(t *testing.T) {
		mockConfig := config.Mock(t)
		mockConfig.Set("logs_config.use_podman_logs", false)

		require.Equal(t,
			filepath.Join(basePath, "123abc/123abc-json.log"),
			getPath("123abc"))
	})
	t.Run("use_podman_logs=true", func(t *testing.T) {
		mockConfig := config.Mock(t)
		mockConfig.Set("logs_config.use_podman_logs", true)

		require.Equal(t,
			"/var/lib/containers/storage/overlay-containers/123abc/userdata/ctr.log",
			getPath("123abc"))
	})
}
