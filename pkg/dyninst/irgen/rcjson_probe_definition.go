// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"errors"
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

// ProbeDefinitionFromRemoteConfig converts a remote config probe into a probe
// definition.
func ProbeDefinitionFromRemoteConfig(
	cfg rcjson.Probe,
) (ProbeDefinition, error) {
	switch cfg := cfg.(type) {
	case *rcjson.LogProbe:
		if cfg.CaptureSnapshot {
			return newSnapshotProbeDefinition(cfg)
		}
		return newLogProbeDefinition(cfg)
	case *rcjson.MetricProbe:
		return newMetricProbeDefinition(cfg)
	case *rcjson.SpanProbe:
		return nil, fmt.Errorf("span probes are not supported")
	default:
		return nil, fmt.Errorf("unexpected rcjson.Probe: %#v", cfg)
	}
}

func newSnapshotProbeDefinition(
	cfg *rcjson.LogProbe,
) (_ *snapshotProbeDefinition, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = fmt.Errorf(
				"invalid snapshot probe (%s@%d): %w",
				cfg.ID, cfg.Version, retErr,
			)
		}
	}()
	if !cfg.CaptureSnapshot {
		return nil, fmt.Errorf("capture_snapshot must be true for snapshot probes")
	}
	if err := validateWhere(cfg.Where); err != nil {
		return nil, err
	}
	return (*snapshotProbeDefinition)(cfg), nil
}

// snapshotProbeDefinition is a probe that captures a snapshot.
type snapshotProbeDefinition rcjson.LogProbe

var _ ProbeDefinition = (*snapshotProbeDefinition)(nil)

func (l *snapshotProbeDefinition) GetID() string         { return l.ID }
func (l *snapshotProbeDefinition) GetVersion() int       { return l.Version }
func (l *snapshotProbeDefinition) GetTags() []string     { return l.Tags }
func (l *snapshotProbeDefinition) GetKind() ir.ProbeKind { return ir.ProbeKindSnapshot }
func (l *snapshotProbeDefinition) GetWhere() Where       { return (*functionWhere)(l.Where) }
func (l *snapshotProbeDefinition) GetCaptureConfig() CaptureConfig {
	return (*captureConfig)(l.Capture)
}
func (l *snapshotProbeDefinition) GetThrottleConfig() ThrottleConfig {
	return (*snapshotThrottleConfig)(l.Sampling)
}

type snapshotThrottleConfig rcjson.Sampling

var _ ThrottleConfig = (*snapshotThrottleConfig)(nil)

func (c *snapshotThrottleConfig) GetThrottlePeriodMs() uint32 {
	return 1000
}

func (c *snapshotThrottleConfig) GetThrottleBudget() int64 {
	if c == nil || c.SnapshotsPerSecond <= 0 {
		return 1
	}
	return int64(c.SnapshotsPerSecond)
}

func newLogProbeDefinition(cfg *rcjson.LogProbe) (_ *logProbeDefinition, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = fmt.Errorf("invalid log probe (%s@%d): %w", cfg.ID, cfg.Version, retErr)
		}
	}()
	if cfg.Template == "" {
		return nil, errors.New("template must be set")
	}
	if len(cfg.Segments) == 0 {
		return nil, errors.New("segments must be set")
	}
	if err := validateWhere(cfg.Where); err != nil {
		return nil, err
	}
	return (*logProbeDefinition)(cfg), nil
}

// logProbeDefinition is a probe that captures a log.
type logProbeDefinition rcjson.LogProbe

var _ ProbeDefinition = (*logProbeDefinition)(nil)

func (l *logProbeDefinition) GetID() string         { return l.ID }
func (l *logProbeDefinition) GetVersion() int       { return l.Version }
func (l *logProbeDefinition) GetTags() []string     { return l.Tags }
func (l *logProbeDefinition) GetKind() ir.ProbeKind { return ir.ProbeKindLog }
func (l *logProbeDefinition) GetWhere() Where       { return (*functionWhere)(l.Where) }
func (l *logProbeDefinition) GetCaptureConfig() CaptureConfig {
	return (*captureConfig)(l.Capture)
}
func (l *logProbeDefinition) GetThrottleConfig() ThrottleConfig {
	return (*logThrottleConfig)(l.Sampling)
}

type logThrottleConfig rcjson.Sampling

// logThrottleConfig is a throttle configuration for log probes that is
var _ ThrottleConfig = (*logThrottleConfig)(nil)

func (c *logThrottleConfig) GetThrottlePeriodMs() uint32 { return 100 }
func (c *logThrottleConfig) GetThrottleBudget() int64    { return 500 }

func validateWhere(where *rcjson.Where) error {
	if where.TypeName != "" {
		return errors.New("type_name must be empty")
	}
	if where.SourceFile != "" && len(where.Lines) > 0 {
		return errors.New("source_file and lines are not currently supported")
	}
	if where.Signature != "" {
		return errors.New("signature must be empty")
	}
	if where.MethodName == "" {
		return errors.New("method_name must be set for probes")
	}
	return nil
}

func newMetricProbeDefinition(
	cfg *rcjson.MetricProbe,
) (_ *metricProbeDefinition, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = fmt.Errorf("invalid metric probe (%s@%d): %w", cfg.ID, cfg.Version, retErr)
		}
	}()
	return (*metricProbeDefinition)(cfg), nil
}

// metricProbeDefinition is a probe that captures a metric.
type metricProbeDefinition rcjson.MetricProbe

var _ ProbeDefinition = (*metricProbeDefinition)(nil)

func (l *metricProbeDefinition) GetID() string         { return l.ID }
func (l *metricProbeDefinition) GetVersion() int       { return l.Version }
func (l *metricProbeDefinition) GetTags() []string     { return l.Tags }
func (l *metricProbeDefinition) GetKind() ir.ProbeKind { return ir.ProbeKindMetric }
func (l *metricProbeDefinition) GetWhere() Where       { return (*functionWhere)(l.Where) }
func (l *metricProbeDefinition) GetCaptureConfig() CaptureConfig {
	return new(metricCaptureConfig)
}
func (l *metricProbeDefinition) GetThrottleConfig() ThrottleConfig {
	return new(metricThrottleConfig)
}

type metricCaptureConfig struct{}

var _ CaptureConfig = (*metricCaptureConfig)(nil)

func (c *metricCaptureConfig) GetMaxReferenceDepth() uint32 { return 0 }
func (c *metricCaptureConfig) GetMaxFieldCount() uint32     { return 0 }
func (c *metricCaptureConfig) GetMaxCollectionSize() uint32 { return 0 }

// metricThrottleConfig returns a limit that is effectively unlimited.
type metricThrottleConfig struct{}

func (c *metricThrottleConfig) GetThrottlePeriodMs() uint32 { return 1000 }
func (c *metricThrottleConfig) GetThrottleBudget() int64    { return math.MaxInt64 }
