// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package createandfetchimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
)

func TestGet(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth_token")
	overrides := map[string]any{
		"auth_token_file_path": authPath,
	}

	comp, err := newAuthToken(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: overrides}),
		),
	)
	require.NoError(t, err)

	data, err := os.ReadFile(authPath)
	require.NoError(t, err)

	assert.Equal(t, string(data), comp.Get())
	assert.Equal(t, util.GetAuthToken(), comp.Get())
}
