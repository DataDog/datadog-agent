package providers

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

type TelemetryProvider struct{}

func NewTelemetryProvider() *TelemetryProvider {
	return &TelemetryProvider{}
}

func (c *TelemetryProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	port := pkgconfig.Datadog.GetString("expvar_port")

	instance := fmt.Sprintf(`
openmetrics_endpoint: http://localhost:%s/telemetry
namespace: datadog.agent
no_index: true
metrics:
- logs_sender_latency.*
- logs_sent.*
- logs_dropped.*
- payload_drops.*`, port)

	return []integration.Config{
		{
			Name:      "openmetrics",
			Instances: []integration.Data{[]byte(instance)},
		},
	}, nil
}

func (c *TelemetryProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return true, nil
}

func (c *TelemetryProvider) String() string {
	return names.Telemetry
}

func (c *TelemetryProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
