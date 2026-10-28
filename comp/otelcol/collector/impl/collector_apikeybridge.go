// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && !aix

package collectorimpl

import (
	"go.opentelemetry.io/collector/otelcol"

	datadogextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"

	"github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/collector/impl/internal/apikeybridge"
)

// bridgeDatadogExtensionAPIKey re-registers the upstream `datadog` extension factory so
// that the extension's private Agent config -- and therefore its forwarder -- follows
// the Agent's API key across a rotation. See comp/otelcol/collector/impl/internal/apikeybridge.
//
// It replaces an existing entry and never inserts one. The `datadog` extension comes from
// the OCB-generated factory map, and a BYOC build may legitimately omit it from its
// manifest (it is not in MANDATORY_COMPONENTS in tasks/collector.py), so adding it back
// here would ship a component the operator deliberately excluded.
func bridgeDatadogExtensionAPIKey(factories otelcol.Factories, cfg config.Component, log corelog.Component) {
	ddExtType := datadogextension.NewFactory().Type()
	if _, ok := factories.Extensions[ddExtType]; !ok {
		return
	}
	factories.Extensions[ddExtType] = datadogextension.NewFactoryWithOptions(
		datadogextension.WithConfigOptions(apikeybridge.Option(cfg, log)),
	)
}
