// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(clusterchecks && kubeapiserver)

package providers

import "github.com/DataDog/datadog-agent/pkg/config"

// NewKubeServiceConfigProvider returns a new ConfigProvider connected to apiserver.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
var NewKubeServiceConfigProvider func(providerConfig *config.ConfigurationProviders) (ConfigProvider, error)
