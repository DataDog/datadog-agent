package agenttelemetryimpl

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type NoopAgentTelemetry struct{}

func (n *NoopAgentTelemetry) SendEvent(eventType string, eventPayload []byte) error {
	return nil
}

func (n *NoopAgentTelemetry) StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context) {
	return &installertelemetry.Span{}, context.Background()
}
