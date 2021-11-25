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
	"go.opentelemetry.io/collector/model/pdata"
	"go.opentelemetry.io/collector/receiver/receiverhelper"
)

const typeStr = "file"

// NewFactory creates a new OTLP receiver factory.
func NewFactory() component.ReceiverFactory {
	return receiverhelper.NewFactory(
		typeStr,
		createDefaultConfig,
		receiverhelper.WithMetrics(createMetricsReceiver),
	)
}

var _ config.Receiver = (*Config)(nil)

// Config of filereceiver.
type Config struct {
	config.ReceiverSettings `mapstructure:",squash"`
	// Path of metrics data.
	Path string `mapstructure:"path"`
	// Delay before starting to send data.
	Delay time.Duration `mapstructure:"delay"`
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
	}
}

var _ component.MetricsReceiver = (*receiver)(nil)

type receiver struct {
	config       *Config
	unmarshaler  pdata.MetricsUnmarshaler
	nextConsumer consumer.Metrics
	stopCh       chan struct{}
}

func (r *receiver) Start(_ context.Context, host component.Host) error {
	go r.unmarshalAndSend(host)
	return nil
}

func (r *receiver) unmarshalAndSend(host component.Host) {
	select {
	case <-time.After(r.config.Delay):
	case <-r.stopCh:
		return
	}

	file, err := os.Open(r.config.Path)
	if err != nil {
		host.ReportFatalError(fmt.Errorf("failed to open %q: %w", r.config.Path, err))
		return
	}

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
		unmarshaler:  otlp.NewJSONMetricsUnmarshaler(),
		nextConsumer: consumer,
		stopCh:       make(chan struct{}),
	}, nil
}
