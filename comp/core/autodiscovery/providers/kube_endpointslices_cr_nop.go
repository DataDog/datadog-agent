// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !(clusterchecks && kubeapiserver)

package providers

import "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"

// NewKubeEndpointSlicesCRConfigProvider is unavailable without cluster checks and the Kubernetes API server.
var NewKubeEndpointSlicesCRConfigProvider types.ConfigProviderFactory
