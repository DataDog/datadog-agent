// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostnameinterface

import hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"

// Re-exported constants from comp/core/hostname/def for backward compatibility.
// Deprecated: import comp/core/hostname/def directly.
const (
	// ConfigProvider is the provider name used when the hostname is found in the configuration file.
	ConfigProvider = hostnamedef.ConfigProvider

	// FargateProvider is the provider name used when running in Fargate/sidecar mode.
	FargateProvider = hostnamedef.FargateProvider
)

// Note: FromConfiguration() and FromFargate() methods are defined on hostnamedef.Data
// and are automatically available on hostnameinterface.Data (type alias).
