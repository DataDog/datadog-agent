// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// newDecoderForSource creates a started decoder for the given source, using
// UTF8 newline framing and a noop parser. The caller is responsible for
// stopping the decoder after the test.
func newDecoderForSource(t *testing.T, src *sources.LogSource) Decoder {
	t.Helper()
	d := NewDecoderWithFraming(
		sources.NewReplaceableSource(src),
		noop.New(),
		framer.UTF8Newline,
		nil,
		status.NewInfoRegistry(),
	)
	d.Start()
	return d
}

// sendAndCount sends n identical newline-terminated messages to d and returns
// how many messages are received from the output channel.
// inputChan and outputChan are unbuffered, so sending and receiving must be
// concurrent to avoid deadlock.
func sendAndCount(t *testing.T, d Decoder, n int) int {
	t.Helper()
	line := []byte("INFO something repetitive happened\n")

	// Send in a separate goroutine so the main goroutine can drain outputChan.
	go func() {
		for range n {
			d.InputChan() <- NewInput(line)
		}
		d.Stop() // closes inputChan → flushes pipeline → closes outputChan
	}()

	count := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-d.OutputChan():
			if !ok {
				return count
			}
			count++
		case <-timeout:
			t.Error("timed out draining output channel")
			return count
		}
	}
}

// TestLogssourceDecoderSeesMoreLogsThanMainAgentWhenSamplingEnabled verifies
// the end-to-end effect of disableAdaptiveSampling (called by logssource on
// every source it registers).
//
// With global adaptive sampling enabled:
//   - A source with ExperimentalAdaptiveSampling=nil (main agent) gets an
//     AdaptiveSampler; repeated identical messages are rate-limited.
//   - A source with ExperimentalAdaptiveSampling.Enabled=false (logssource
//     after the fix) gets a NoopSampler; every message passes through.
//
// With global adaptive sampling disabled, both sources use NoopSampler and
// both pipelines receive the same count.
func TestLogssourceDecoderSeesMoreLogsThanMainAgentWhenSamplingEnabled(t *testing.T) {
	const N = 20

	// Use burst=1, rate=0: exactly one log gets through per pattern, then all
	// subsequent identical messages are dropped regardless of elapsed time.
	cfg := configmock.New(t)
	cfg.Set("logs_config.experimental_adaptive_sampling.burst_size", 1.0, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("logs_config.experimental_adaptive_sampling.rate_limit", 0.0, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("logs_config.experimental_adaptive_sampling.max_patterns", 10, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("logs_config.experimental_adaptive_sampling.match_threshold", 0.8, pkgconfigmodel.SourceAgentRuntime)

	disabled := false

	mainAgentSource := sources.NewLogSource("main-agent", &config.LogsConfig{
		// No ExperimentalAdaptiveSampling override: inherits global flag.
	})
	logssourceSource := sources.NewLogSource("logssource", &config.LogsConfig{
		// Mirrors what disableAdaptiveSampling stamps on every logssource source.
		ExperimentalAdaptiveSampling: &config.SourceAdaptiveSamplingOptions{
			Enabled: &disabled,
		},
	})

	t.Run("sampling enabled: logssource sees all logs, main agent drops most", func(t *testing.T) {
		cfg.Set("logs_config.experimental_adaptive_sampling.enabled", true, pkgconfigmodel.SourceAgentRuntime)

		mainCount := sendAndCount(t, newDecoderForSource(t, mainAgentSource), N)
		logssourceCount := sendAndCount(t, newDecoderForSource(t, logssourceSource), N)

		assert.Equal(t, N, logssourceCount,
			"logssource decoder must pass all %d logs through (NoopSampler)", N)
		require.Less(t, mainCount, N,
			"main agent decoder must drop some logs (AdaptiveSampler with burst=1)")
		assert.Greater(t, logssourceCount, mainCount,
			"logssource must receive more logs than the main agent pipeline")
	})

	t.Run("sampling disabled: both pipelines receive all logs", func(t *testing.T) {
		cfg.Set("logs_config.experimental_adaptive_sampling.enabled", false, pkgconfigmodel.SourceAgentRuntime)

		mainCount := sendAndCount(t, newDecoderForSource(t, mainAgentSource), N)
		logssourceCount := sendAndCount(t, newDecoderForSource(t, logssourceSource), N)

		assert.Equal(t, N, mainCount,
			"main agent decoder must pass all logs when sampling is disabled")
		assert.Equal(t, N, logssourceCount,
			"logssource decoder must pass all logs when sampling is disabled")
	})
}
