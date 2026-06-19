// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
)

func TestRegisterProvidersDoesNotRegisterCloudFoundryInKubeAPIServerBuild(t *testing.T) {
	providerCatalog := map[string]types.ConfigProviderFactory{}

	RegisterProviders(providerCatalog)

	require.NotContains(t, providerCatalog, names.CloudFoundryBBS)
}
