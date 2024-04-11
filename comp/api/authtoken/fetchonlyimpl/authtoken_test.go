// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package fetchonlyimpl

import (
	"os"
	"path/filepath"
	"testing"

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

	comp := newAuthToken(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: overrides}),
		),
	).(*authToken)

	assert.Empty(t, comp.Get())
	assert.False(t, comp.tokenLoaded)

	err := os.WriteFile(authPath, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0777)
	require.NoError(t, err)

	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", comp.Get())
	assert.True(t, comp.tokenLoaded)

}
