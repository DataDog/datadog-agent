// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package adaptivesampling_test

import (
	"hash/fnv"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/preprocessor"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestSyntheticHighLogPatternAnomalyBoostsAdaptiveSampler(t *testing.T) {
	adaptivesampling.ResetDefaultSamplingBoostStoreForTest()
	t.Cleanup(adaptivesampling.ResetDefaultSamplingBoostStoreForTest)

	const (
		containerID = "container-a"
		content     = "request completed id=123 duration=42ms"
	)
	tokens := tokenize(content)
	patternHash := samplerPatternHash(tokens)
	sampler := preprocessor.NewAdaptiveSampler(preprocessor.AdaptiveSamplerConfig{
		MaxPatterns:    10,
		RateLimit:      0,
		BurstSize:      1,
		MatchThreshold: 0.9,
	}, "test", 0)

	require.NotNil(t, sampler.Process(testMsg(content, containerID), tokens), "first log creates the pattern and passes")
	assert.Nil(t, sampler.Process(testMsg(content, containerID), tokens), "second matching log is suppressed before boost")

	score := 20.0 // holt_residual level 3 (high) with DefaultScorerConfig.
	boosted := observerimpl.MaybeEmitSamplingBoostForAnomaly(observerdef.Anomaly{
		Source: observerdef.SeriesDescriptor{
			Namespace: observerimpl.LogMetricsExtractorName,
			Name:      "log.pattern." + patternHash + ".count",
		},
		DetectorName: "holt_residual",
		Score:        &score,
		Context: &observerdef.MetricContext{
			ContainerID: containerID,
			PatternHash: patternHash,
		},
	}, adaptivesampling.DefaultSamplingBoostStore(), observerimpl.DefaultScorerConfig(), time.Now())
	require.True(t, boosted, "synthetic high log-pattern anomaly should emit a boost")

	out := sampler.Process(testMsg(content, containerID), tokens)
	require.NotNil(t, out, "boost should let the next matching log pass")
	assert.True(t, hasTagPrefix(out.ParsingExtra.Tags, "adaptive_sampler_sampled_count:"),
		"passed log should report the count suppressed before the boost")
}

func tokenize(content string) []preprocessor.Token {
	tok := preprocessor.NewTokenizer(0)
	tokens, _ := tok.Tokenize([]byte(content))
	return tokens
}

func samplerPatternHash(tokens []preprocessor.Token) string {
	var b [1]byte
	h := fnv.New64a()
	for _, token := range tokens {
		b[0] = byte(token)
		_, _ = h.Write(b[:])
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

func testMsg(content, containerID string) *message.Message {
	source := sources.NewLogSource("test", &logsconfig.LogsConfig{
		Identifier: containerID,
	})
	return message.NewMessage([]byte(content), message.NewOrigin(source), message.StatusInfo, time.Now().UnixNano())
}

func hasTagPrefix(tags []string, prefix string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			return true
		}
	}
	return false
}
