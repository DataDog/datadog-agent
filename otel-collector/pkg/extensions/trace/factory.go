package trace

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"go.uber.org/zap"

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

// zaplogger implements the tracelog.Logger interface on top of a zap.Logger
type zaplogger struct{ logger *zap.Logger }

// Trace implements Logger.
func (z *zaplogger) Trace(v ...interface{}) { /* N/A */ }

// Tracef implements Logger.
func (z *zaplogger) Tracef(format string, params ...interface{}) { /* N/A */ }

// Debug implements Logger.
func (z *zaplogger) Debug(v ...interface{}) {
	z.logger.Debug(fmt.Sprint(v...))
}

// Debugf implements Logger.
func (z *zaplogger) Debugf(format string, params ...interface{}) {
	z.logger.Debug(fmt.Sprintf(format, params...))
}

// Info implements Logger.
func (z *zaplogger) Info(v ...interface{}) {
	z.logger.Info(fmt.Sprint(v...))
}

// Infof implements Logger.
func (z *zaplogger) Infof(format string, params ...interface{}) {
	z.logger.Info(fmt.Sprintf(format, params...))
}

// Warn implements Logger.
func (z *zaplogger) Warn(v ...interface{}) error {
	z.logger.Warn(fmt.Sprint(v...))
	return nil
}

// Warnf implements Logger.
func (z *zaplogger) Warnf(format string, params ...interface{}) error {
	z.logger.Warn(fmt.Sprintf(format, params...))
	return nil
}

// Error implements Logger.
func (z *zaplogger) Error(v ...interface{}) error {
	z.logger.Error(fmt.Sprint(v...))
	return nil
}

// Errorf implements Logger.
func (z *zaplogger) Errorf(format string, params ...interface{}) error {
	z.logger.Error(fmt.Sprintf(format, params...))
	return nil
}

// Critical implements Logger.
func (z *zaplogger) Critical(v ...interface{}) error {
	z.logger.Error(fmt.Sprint(v...), zap.Bool("critical", true))
	return nil
}

// Criticalf implements Logger.
func (z *zaplogger) Criticalf(format string, params ...interface{}) error {
	z.logger.Error(fmt.Sprintf(format, params...), zap.Bool("critical", true))
	return nil
}

// Flush implements Logger.
func (z *zaplogger) Flush() {
	_ = z.logger.Sync()
}
