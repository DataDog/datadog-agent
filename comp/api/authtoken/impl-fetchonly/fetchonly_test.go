// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package fetchonlyimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/api/security/cert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func TestGet(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth_token")

	configComp := config.NewMock(t)
	configComp.SetWithoutSource("auth_token_file_path", authPath)
	logComp := logmock.New(t)

	requires := Requires{
		Conf: configComp,
		Log:  logComp,
	}

	provider := NewComponent(requires)

	comp := provider.Comp.(*authToken)

	assert.Empty(t, comp.Get())
	assert.False(t, comp.tokenLoaded)

	err := os.WriteFile(authPath, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0777)
	require.NoError(t, err)

	// Should be empty because the cert/key weren't generated yet
	assert.Empty(t, comp.Get())
	assert.False(t, comp.tokenLoaded)

	// generating IPC cert/key files
	_, _, err = cert.CreateOrFetchAgentIPCCert(configComp)
	require.NoError(t, err)

	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", comp.Get())
	assert.True(t, comp.tokenLoaded)

}
