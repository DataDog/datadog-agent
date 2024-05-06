// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet

package providers

import "github.com/DataDog/datadog-agent/pkg/config"

// NewEndpointsChecksConfigProvider returns a new ConfigProvider collecting
// endpoints check configurations from the cluster-agent.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
var NewEndpointsChecksConfigProvider func(providerConfig *config.ConfigurationProviders) (ConfigProvider, error)
