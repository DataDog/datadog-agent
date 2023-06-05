package trace

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"

	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

const typeStr = "trace"

// NewFactory creates a factory
func NewFactory() extension.Factory {
	return extension.NewFactory(
		typeStr,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelAlpha)
}

func createDefaultConfig() component.Config {
	return &Config{
		agentConf: config.New(),
	}
}

func createExtension(
	ctx context.Context,
	params extension.CreateSettings,
	cfg component.Config,
) (extension.Extension, error) {
	tracelog.SetLogger(&zaplogger{logger: params.Logger})
	cg := cfg.(*Config)
	return newTraceAgent(ctx, cg)
}

func newTraceAgent(ctx context.Context, cfg *Config) (*traceAgent, error) {
	ag := agent.NewAgent(ctx, cfg.agentConf, telemetry.NewNoopCollector())
	return &traceAgent{
		agent:  ag,
		config: cfg,
	}, nil
}
