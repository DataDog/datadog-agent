// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
)

// TestShouldUseDirectConsumer verifies that the DirectConsumer is only selected when the
// loaded eBPF object cannot be the prebuilt one (which has the Kafka direct-emit path
// compiled out). In particular, with prebuilt fallback enabled a CO-RE/runtime load failure
// could silently drop to prebuilt, so Kafka must use the batch consumer in that case.
func TestShouldUseDirectConsumer(t *testing.T) {
	if !events.SupportsDirectConsumer() {
		t.Skip("direct consumer requires kernel >= 5.8.0")
	}

	// CO-RE enabled, prebuilt fallback disabled: prebuilt can never load, so direct is safe.
	newCfg := func() *config.Config {
		c := config.New()
		c.KafkaUseDirectConsumer = true
		c.EnableCORE = true
		c.EnableRuntimeCompiler = false
		c.AllowPrebuiltFallback = false
		return c
	}

	require.True(t, shouldUseDirectConsumer(newCfg()),
		"CO-RE enabled and prebuilt fallback disabled: prebuilt cannot load, direct is safe")

	cfgFallback := newCfg()
	cfgFallback.AllowPrebuiltFallback = true
	require.False(t, shouldUseDirectConsumer(cfgFallback),
		"prebuilt fallback enabled: a CO-RE/runtime load failure could drop to the prebuilt object (direct path compiled out), so use the batch consumer")

	cfgPrebuiltOnly := newCfg()
	cfgPrebuiltOnly.EnableCORE = false
	cfgPrebuiltOnly.EnableRuntimeCompiler = false
	require.False(t, shouldUseDirectConsumer(cfgPrebuiltOnly),
		"neither CO-RE nor runtime enabled: prebuilt is the only option, use the batch consumer")

	cfgNotRequested := newCfg()
	cfgNotRequested.KafkaUseDirectConsumer = false
	require.False(t, shouldUseDirectConsumer(cfgNotRequested),
		"direct consumer not requested: use the batch consumer")
}
