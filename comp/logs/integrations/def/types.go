// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrations

import "github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"

// IntegrationLog represents the combined Log and IntegrationID for the
// integration sending the log
type IntegrationLog struct {
	Log           string
	IntegrationID string
}

// IntegrationConfig represents the combined ID and Config for an integration
type IntegrationConfig struct {
	IntegrationID string
	Config        integration.Config
}
