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
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func TestGet(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth_token")
	var cfg config.Component
	overrides := map[string]any{
		"auth_token_file_path": authPath,
	}

	comp := newAuthToken(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Populate(&cfg),
			fx.Replace(config.MockParams{Overrides: overrides}),
		),
	).(*authToken)

	_, err := comp.Get()

	assert.Error(t, err)
	assert.False(t, comp.tokenLoaded)

	err = os.WriteFile(authPath, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0777)
	require.NoError(t, err)

	// Should be empty because the cert/key weren't generated yet
	_, err = comp.Get()
	assert.Error(t, err)
	assert.False(t, comp.tokenLoaded)

	// generating IPC cert/key files
	_, _, err = cert.CreateOrFetchAgentIPCCert(cfg)
	require.NoError(t, err)

	token, err := comp.Get()
	assert.NoError(t, err)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", token)
	assert.True(t, comp.tokenLoaded)

}
