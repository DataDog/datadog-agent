// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && aix

package collectorimpl

import (
	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// bridgeDatadogExtensionAPIKey does nothing on aix. The extension's WithConfigOptions
// takes a pkg/datadog/agentcomponents type that does not build there, and the extension
// itself is unsupported -- its NewFactory panics -- so there is no forwarder to follow
// the Agent's API key.
func bridgeDatadogExtensionAPIKey(_ otelcol.Factories, _ config.Component, _ corelog.Component) {}
