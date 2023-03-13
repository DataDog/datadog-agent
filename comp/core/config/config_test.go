// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRealConfig(t *testing.T) {
	// point the ConfFilePath to a valid, but empty config file so that it does
	// not use the config file on the developer's system
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("{}"), 0666)

	os.Setenv("DD_DD_URL", "https://example.com")
	defer func() { os.Unsetenv("DD_DD_URL") }()

	fxutil.Test(t, fx.Options(
		fx.Supply(NewParams(
			"",
			WithConfigMissingOK(true),
			WithConfFilePath(dir),
		)),
		Module,
	), func(config Component) {
		require.Equal(t, "https://example.com", config.GetString("dd_url"))
	})
}

// TODO: test various bundle params
