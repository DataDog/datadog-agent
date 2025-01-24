//go:build serverless

package providers

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

var NewGPUConfigProvider func(providerConfig *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, telemetryStore *telemetry.Store) (ConfigProvider, error)
