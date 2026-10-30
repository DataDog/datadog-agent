// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package rpm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

func TestNewCentOSBackendRejectsPre8(t *testing.T) {
	for _, release := range []string{"CentOS release 6.10 (Final)", "CentOS Linux release 7.9.2009 (Core)"} {
		t.Run(release, func(t *testing.T) {
			hostEtc := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(hostEtc, "redhat-release"), []byte(release+"\n"), 0o644))
			t.Setenv("HOST_ETC", hostEtc)

			_, err := NewCentOSBackend(&types.Target{}, t.TempDir(), nil)
			require.ErrorContains(t, err, "kernel header download is not supported on CentOS")
		})
	}
}
