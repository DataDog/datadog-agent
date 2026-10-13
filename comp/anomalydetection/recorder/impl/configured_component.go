// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"context"
	"errors"
	"fmt"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires lists the recorder's configuration and optional dependencies.
type Requires struct {
	Config    config.Component
	Lifecycle compdef.Lifecycle `optional:"true"`
	Factory   option.Option[recorder.WriterFactory]
}

// Provides contains the optional recorder component.
type Provides struct {
	Comp option.Option[recorder.Component]
}

// NewConfiguredComponent constructs writers only when recording and a provider
// are both available.
func NewConfiguredComponent(req Requires) (Provides, error) {
	none := Provides{Comp: option.None[recorder.Component]()}
	if !req.Config.GetBool("anomaly_detection.recording.enabled") {
		return none, nil
	}
	factory, present := req.Factory.Get()
	if !present {
		return none, nil
	}
	if factory == nil {
		return Provides{}, errors.New("recorder writer factory is nil")
	}

	cfg, err := writerConfig(req.Config)
	if err != nil {
		return Provides{}, err
	}
	if req.Lifecycle == nil {
		return Provides{}, errors.New("recorder lifecycle not set")
	}

	metrics, err := factory.NewMetricWriter(cfg)
	if err != nil {
		return Provides{}, fmt.Errorf("creating metric writer: %w", err)
	}
	logs, err := factory.NewLogWriter(cfg)
	if err != nil {
		return Provides{}, errors.Join(
			fmt.Errorf("creating log writer: %w", err),
			closeMetricWriter(metrics),
		)
	}

	req.Lifecycle.Append(compdef.Hook{OnStop: func(context.Context) error {
		return errors.Join(closeMetricWriter(metrics), closeLogWriter(logs))
	}})
	return Provides{Comp: option.New(NewComponent(metrics, logs))}, nil
}

func closeMetricWriter(writer recorder.MetricWriter) error {
	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing metric writer: %w", err)
	}
	return nil
}

func closeLogWriter(writer recorder.LogWriter) error {
	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing log writer: %w", err)
	}
	return nil
}
