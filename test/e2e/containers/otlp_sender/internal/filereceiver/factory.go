// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package filereceiver implements a receiver that reads OTLP metrics from a given file.
package filereceiver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/model/otlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

const typeStr = "file"

// NewFactory creates a new OTLP receiver factory.
func NewFactory() component.ReceiverFactory {
	return component.NewReceiverFactory(
		typeStr,
		createDefaultConfig,
		component.WithMetricsReceiver(createMetricsReceiver),
	)
}

var _ config.Receiver = (*Config)(nil)

// Config of filereceiver.
type Config struct {
	config.ReceiverSettings `mapstructure:",squash"`
	// Path of metrics data.
	Path string `mapstructure:"path"`
	// LoopConfig is the loop configuration.
	Loop LoopConfig `mapstructure:"loop"`
}

// LoopConfig is the loop configuration.
type LoopConfig struct {
	// Enabled states whether the feature is enabled.
	Enabled bool `mapstructure:"enabled"`
	// Period defines the loop period.
	Period time.Duration `mapstructure:"period"`
}

// Validate configuration of receiver.
func (cfg *Config) Validate() error {
	if cfg.Path == "" {
		return errors.New("path can't be empty")
	}
	return nil
}

func createDefaultConfig() config.Receiver {
	return &Config{
		ReceiverSettings: config.NewReceiverSettings(config.NewComponentID(typeStr)),
		Loop:             LoopConfig{Enabled: false, Period: 10 * time.Second},
	}
}

var _ component.MetricsReceiver = (*receiver)(nil)

type receiver struct {
	config       *Config
	logger       *zap.Logger
	unmarshaler  pmetric.Unmarshaler
	nextConsumer consumer.Metrics
	stopCh       chan struct{}
}

func (r *receiver) Start(_ context.Context, host component.Host) error {
	if r.config.Loop.Enabled {
		r.logger.Info("Running in a loop")
		go r.unmarshalLoop(host)
	} else {
		r.logger.Info("Running just once")
		go r.unmarshalAndSend(host)
	}
	return nil
}

func (r *receiver) unmarshalAndSend(host component.Host) {
	file, err := os.Open(r.config.Path)
	if err != nil {
		host.ReportFatalError(fmt.Errorf("failed to open %q: %w", r.config.Path, err))
		return
	}

	r.logger.Info("Sending metrics batch")
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		metrics, err := r.unmarshaler.UnmarshalMetrics(scanner.Bytes())
		if err != nil {
			host.ReportFatalError(fmt.Errorf("failed to unmarshal %q: %w", r.config.Path, err))
			return
		}

		err = r.nextConsumer.ConsumeMetrics(context.Background(), metrics)
		if err != nil {
			host.ReportFatalError(fmt.Errorf("failed to send %q: %w", r.config.Path, err))
			return
		}
	}

	if err := scanner.Err(); err != nil {
		host.ReportFatalError(fmt.Errorf("failed to scan %q: %w", r.config.Path, err))
		return
	}

	if err := file.Close(); err != nil {
		host.ReportFatalError(fmt.Errorf("failed to close %q: %w", r.config.Path, err))
		return
	}
}

func (r *receiver) unmarshalLoop(host component.Host) {
	for {
		r.unmarshalAndSend(host)
		select {
		case <-time.After(r.config.Loop.Period):
		case <-r.stopCh:
			return
		}
	}
}

func (r *receiver) Shutdown(context.Context) error {
	close(r.stopCh)
	return nil
}

func createMetricsReceiver(
	_ context.Context,
	set component.ReceiverCreateSettings,
	cfg config.Receiver,
	consumer consumer.Metrics,
) (component.MetricsReceiver, error) {
	return &receiver{
		config:       cfg.(*Config),
		logger:       set.Logger,
		unmarshaler:  otlp.NewJSONMetricsUnmarshaler(),
		nextConsumer: consumer,
		stopCh:       make(chan struct{}),
	}, nil
}
