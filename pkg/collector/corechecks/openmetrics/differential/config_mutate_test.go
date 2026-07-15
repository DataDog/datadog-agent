// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigMutatorUsesFixtureVocabulary(t *testing.T) {
	ksm := NewConfigMutatorForFixture(1, "ksm/wildcard")
	require.Contains(t, kubeStateMetricNames, ksm.pickMetricName())
	require.Contains(t, kubeStateLabelNames, ksm.pickLabelName())

	msk := NewConfigMutatorForFixture(1, "msk_jmx/wildcard")
	require.Contains(t, mskMetricNames, msk.pickMetricName())
	require.Contains(t, mskLabelNames, msk.pickLabelName())

	lading := NewConfigMutatorForFixture(1, ladingFixtureName)
	require.Contains(t, ladingMetricNames, lading.pickMetricName())
	require.Contains(t, ladingLabelNames, lading.pickLabelName())
}

func TestShareLabelsKnobUsesSourceMetricAndJoinLabels(t *testing.T) {
	cfg := baseConfig()
	knobShareLabels(NewConfigMutator(1), cfg)

	shareLabels, ok := cfg["share_labels"].(map[string]interface{})
	require.True(t, ok)

	ksmSource, ok := shareLabels["kube_pod_info"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"namespace", "pod"}, ksmSource["match"])
	require.Equal(t, []interface{}{"node", "pod_ip"}, ksmSource["labels"])

	mskSource, ok := shareLabels["kafka_server_FetcherStats_FiveMinuteRate"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"clientId", "name"}, mskSource["match"])
	require.Equal(t, []interface{}{"brokerHost", "brokerPort"}, mskSource["labels"])

	ladingSource, ok := shareLabels["diff_lading_target_info"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"service", "region"}, ladingSource["match"])
	require.Equal(t, []interface{}{"shard"}, ladingSource["labels"])
}
