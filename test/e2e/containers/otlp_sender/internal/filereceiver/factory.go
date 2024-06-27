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
	"log"
	"os"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	collectorreceiver "go.opentelemetry.io/collector/receiver"

	"go.uber.org/zap"
)

const typeStr = "file"

// NewFactory creates a new OTLP receiver factory.
func NewFactory() collectorreceiver.Factory {
	cfgType, _ := component.NewType(typeStr)
	return collectorreceiver.NewFactory(
		cfgType,
		createDefaultConfig,
		collectorreceiver.WithMetrics(createMetricsReceiver, component.StabilityLevelAlpha),
	)
}

// Config of filereceiver.
type Config struct {
	collectorreceiver.Settings `mapstructure:",squash"`
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

func createDefaultConfig() component.Config {
	cfgType, _ := component.NewType(typeStr)
	return &Config{
		Settings: collectorreceiver.Settings{
			ID: component.NewID(cfgType),
		},
		Loop: LoopConfig{Enabled: false, Period: 10 * time.Second},
	}
}

var _ collectorreceiver.Metrics = (*receiver)(nil)

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

func (r *receiver) unmarshalAndSend(_ component.Host) {
	file, err := os.Open(r.config.Path)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to open %q: %w", r.config.Path, err))
		return
	}

	r.logger.Info("Sending metrics batch")
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		metrics, err := r.unmarshaler.UnmarshalMetrics(scanner.Bytes())
		if err != nil {
			log.Fatal(fmt.Errorf("failed to unmarshal %q: %w", r.config.Path, err))
			return
		}

		err = r.nextConsumer.ConsumeMetrics(context.Background(), metrics)
		if err != nil {
			log.Fatal(fmt.Errorf("failed to send %q: %w", r.config.Path, err))
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(fmt.Errorf("failed to scan %q: %w", r.config.Path, err))
		return
	}

	if err := file.Close(); err != nil {
		log.Fatal(fmt.Errorf("failed to close %q: %w", r.config.Path, err))
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
	set collectorreceiver.Settings,
	cfg component.Config,
	consumer consumer.Metrics,
) (collectorreceiver.Metrics, error) {
	return &receiver{
		config:       cfg.(*Config),
		logger:       set.Logger,
		unmarshaler:  &pmetric.JSONUnmarshaler{},
		nextConsumer: consumer,
		stopCh:       make(chan struct{}),
	}, nil
}
