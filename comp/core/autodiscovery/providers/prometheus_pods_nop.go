// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet

package providers

import "github.com/DataDog/datadog-agent/pkg/config"

// NewPrometheusPodsConfigProvider returns a new Prometheus ConfigProvider connected to kubelet.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
var NewPrometheusPodsConfigProvider func(providerConfig *config.ConfigurationProviders) (ConfigProvider, error)
