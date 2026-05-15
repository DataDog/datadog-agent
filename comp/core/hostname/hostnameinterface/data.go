// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def instead.
package hostnameinterface

import hostnameinterfacedef "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"

const (
	// ConfigProvider is the default provider value from the configuration file
	//
	// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def.ConfigProvider instead.
	ConfigProvider = hostnameinterfacedef.ConfigProvider

	// FargateProvider is the default provider value from Fargate
	//
	// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def.FargateProvider instead.
	FargateProvider = hostnameinterfacedef.FargateProvider
)
