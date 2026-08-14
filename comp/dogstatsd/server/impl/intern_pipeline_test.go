// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// TestInternedTagsReachTheSample checks that tags parsed off the wire arrive at
// the aggregator as handles rather than being copied into plain strings, and that
// the handles are canonical: the same tag seen on two different messages resolves
// to the same handle, so the aggregator holds one copy of the string.
func TestInternedTagsReachTheSample(t *testing.T) {
	conf := enrichConfig{defaultHostname: "default-hostname"}

	first, err := parseAndEnrichMultipleMetricMessageNoResolve(t,
		[]byte("daemon:666|g|#env:prod,service:api"), conf)
	require.NoError(t, err)
	require.Len(t, first, 1)

	second, err := parseAndEnrichMultipleMetricMessageNoResolve(t,
		[]byte("other:1|c|#env:prod,service:api"), conf)
	require.NoError(t, err)
	require.Len(t, second, 1)

	// The pipeline populates ITags, not Tags.
	require.Len(t, first[0].ITags, 2)
	assert.Nil(t, first[0].Tags, "the interned pipeline must not materialize Tags")

	assert.Equal(t, []string{"env:prod", "service:api"}, tagset.Values(first[0].ITags))

	// Handles are canonical across parsers and messages.
	assert.Equal(t, first[0].ITags, second[0].ITags,
		"identical tags on different messages must share handles")

	// The memoized hash matches what the accumulator would compute for the string.
	for _, itag := range first[0].ITags {
		assert.Equal(t, tagset.Intern(itag.Value()).Hash(), itag.Hash())
	}
}

// TestInternedTagsFeedAccumulator checks the aggregator side: GetTags must push
// the handles into the hashing accumulator with their precomputed hashes, and the
// resulting tag set must be identical to the one a plain-string sample produces.
func TestInternedTagsFeedAccumulator(t *testing.T) {
	conf := enrichConfig{defaultHostname: "default-hostname"}

	samples, err := parseAndEnrichMultipleMetricMessageNoResolve(t,
		[]byte("daemon:666|g|#env:prod,service:api"), conf)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	interned := tagset.NewHashingTagsAccumulator()
	interned.AppendInterned(samples[0].ITags...)

	plain := tagset.NewHashingTagsAccumulatorWithTags([]string{"env:prod", "service:api"})

	assert.Equal(t, plain.Get(), interned.Get())
	assert.Equal(t, plain.Hash(), interned.Hash(),
		"interned tags must hash identically to the same tags as strings")
}

func parseAndEnrichMultipleMetricMessageNoResolve(t *testing.T, message []byte, conf enrichConfig) ([]metrics.MetricSample, error) {
	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	parser := newParser(deps.Config, newFloat64ListPool(deps.Config, deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	parsed, err := parser.parseMetricSample(message)
	if err != nil {
		return nil, err
	}
	return enrichMetricSample(nil, parsed, "", 0, "", conf, nil), nil
}
