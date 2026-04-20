// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package cnm implements an OTel receiver that collects network connection data
// via eBPF and produces pmetric.Metrics for the CNM (Cloud Networking Monitoring) pipeline.
package cnm

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
)

// NewFactory creates an OTel receiver factory for the CNM receiver in standalone mode (no Agent Core).
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		component.MustNewType("cnm"),
		defaultConfig,
		receiver.WithMetrics(createStandaloneMetricsReceiver, component.StabilityLevelAlpha),
	)
}

// NewFactoryForAgent creates an OTel receiver factory for the CNM receiver in Agent Core mode,
// with access to the tagger, hostname, config, and log components.
func NewFactoryForAgent(
	tagger tagger.Component,
	hostname hostname.Component,
	config coreconfig.Component,
	log log.Component,
) receiver.Factory {
	f := &cnmFactory{
		tagger:   tagger,
		hostname: hostname,
		config:   config,
		log:      log,
	}
	return receiver.NewFactory(
		component.MustNewType("cnm"),
		defaultConfig,
		receiver.WithMetrics(f.createAgentMetricsReceiver, component.StabilityLevelAlpha),
	)
}

// cnmFactory holds Agent Core dependencies for creating CNM receivers.
type cnmFactory struct {
	tagger   tagger.Component
	hostname hostname.Component
	config   coreconfig.Component
	log      log.Component
}

func createStandaloneMetricsReceiver(
	_ context.Context,
	settings receiver.Settings,
	baseCfg component.Config,
	nextConsumer consumer.Metrics,
) (receiver.Metrics, error) {
	cfg, err := castConfig(baseCfg)
	if err != nil {
		return nil, err
	}
	return newCNMReceiver(cfg, settings.Logger, nextConsumer, nil, nil, nil), nil
}

func (f *cnmFactory) createAgentMetricsReceiver(
	_ context.Context,
	settings receiver.Settings,
	baseCfg component.Config,
	nextConsumer consumer.Metrics,
) (receiver.Metrics, error) {
	cfg, err := castConfig(baseCfg)
	if err != nil {
		return nil, err
	}
	return newCNMReceiver(cfg, settings.Logger, nextConsumer, f.tagger, f.hostname, f.config), nil
}

func newCNMReceiver(
	cfg *Config,
	logger *zap.Logger,
	consumer consumer.Metrics,
	tagger tagger.Component,
	hostname hostname.Component,
	agentCfg coreconfig.Component,
) *cnmReceiver {
	return &cnmReceiver{
		cfg:      cfg,
		logger:   logger,
		consumer: consumer,
		tagger:   tagger,
		hostname: hostname,
		agentCfg: agentCfg,
	}
}
