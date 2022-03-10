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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestGetPath(t *testing.T) {
	t.Run("runtime=Docker", func(t *testing.T) {
		l := &Launcher{runtime: config.Docker}
		require.Equal(t,
			filepath.Join(basePath, "123abc/123abc-json.log"),
			l.getContainerLogfilePath("123abc"))
	})
	t.Run("runtime=Podman", func(t *testing.T) {
		l := &Launcher{runtime: config.Podman}
		require.Equal(t,
			"/var/lib/containers/storage/overlay-containers/123abc/userdata/ctr.log",
			l.getContainerLogfilePath("123abc"))
	})
}
