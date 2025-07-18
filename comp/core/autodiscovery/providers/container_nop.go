// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package providers

import "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"

// NewContainerConfigProvider returns a new ConfigProvider subscribed to both container
// and pods
var NewContainerConfigProvider types.ConfigProviderFactory
