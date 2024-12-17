// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package createandfetchimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/api/util"
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

	provider, err := NewComponent(requires)

	require.NoError(t, err)

	comp := provider.Comp

	data, err := os.ReadFile(authPath)
	require.NoError(t, err)

	assert.Equal(t, string(data), comp.Get())
	assert.Equal(t, util.GetAuthToken(), comp.Get())
}
