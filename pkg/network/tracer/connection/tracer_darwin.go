// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package connection

import (
	"fmt"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewTracer returns a new Tracer for Darwin
// The private NStat backend is opt-in and falls back to the existing
// eBPF-less implementation when its kernel control is unavailable.
func NewTracer(cfg *config.Config, _ telemetry.Component) (Tracer, error) {
	return newDarwinTracer(
		cfg,
		func() (Tracer, error) { return newEbpfLessTracer(cfg) },
		func() (Tracer, error) {
			return newDarwinCompositeTracer(darwinCompositeConfig(cfg))
		},
	)
}

func newDarwinTracer(cfg *config.Config, newEbpfless, newComposite func() (Tracer, error)) (Tracer, error) {
	switch cfg.DarwinConnectionTracerBackend {
	case "", config.DarwinConnectionTracerEbpfless:
		return newEbpfless()
	case config.DarwinConnectionTracerNStat,
		config.DarwinConnectionTracerNStatPcap,
		config.DarwinConnectionTracerAuto:
		tracer, err := newComposite()
		if err == nil {
			return newDarwinRuntimeFallbackTracer(tracer, newEbpfless), nil
		}
		log.Warnf("Darwin composite connection tracer unavailable, falling back to eBPF-less tracing: %v", err)
		return newEbpfless()
	default:
		return nil, fmt.Errorf("unknown Darwin connection tracer backend %q", cfg.DarwinConnectionTracerBackend)
	}
}

// darwinCompositeConfig copies cfg and applies backend packet-enrichment policy.
// nstat forces packet capture off. auto is complete nstat-pcap and forces it on.
// nstat-pcap keeps the packet_enabled kill switch. The caller's cfg is not mutated.
func darwinCompositeConfig(cfg *config.Config) *config.Config {
	compositeCfg := *cfg
	switch cfg.DarwinConnectionTracerBackend {
	case config.DarwinConnectionTracerNStat:
		compositeCfg.DarwinConnectionTracerPacketEnabled = false
	case config.DarwinConnectionTracerAuto:
		compositeCfg.DarwinConnectionTracerPacketEnabled = true
	}
	return &compositeCfg
}
