// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && (aix || freebsd || solaris)

package collectorimpl

import (
	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// bridgeDatadogExtensionAPIKey does nothing on the platforms where the upstream extension
// is unsupported: datadogextension.WithConfigOptions does not exist there (its
// factory_unsupported.go omits it), and the extension never starts anyway, so there is no
// forwarder to follow the Agent's API key.
func bridgeDatadogExtensionAPIKey(_ otelcol.Factories, _ config.Component, _ corelog.Component) {}
