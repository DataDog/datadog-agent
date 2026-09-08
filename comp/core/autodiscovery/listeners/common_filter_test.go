// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestNamespaceRoot(t *testing.T) {
	cases := []struct {
		name      string
		namespace string
		want      string
	}{
		{"no dot", "haproxy", "haproxy"},
		{"rooted namespace", "krakend.api", "krakend"},
		{"multiple dots roots at the first one", "a.b.c", "a"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NamespaceRoot(tc.namespace))
		})
	}
}

func TestExpectedNamespaceRoot(t *testing.T) {
	cases := []struct {
		name string
		cfg  integration.Config
		want string
	}{
		{
			"no Discovery falls back to check name",
			integration.Config{Name: "krakend"},
			"krakend",
		},
		{
			"Discovery with empty MetricsPrefix falls back to check name",
			integration.Config{Name: "krakend", Discovery: &integration.DiscoveryConfig{}},
			"krakend",
		},
		{
			"MetricsPrefix rooting to the same value as the check name: no behavior change",
			integration.Config{Name: "krakend", Discovery: &integration.DiscoveryConfig{MetricsPrefix: "krakend.api"}},
			"krakend",
		},
		{
			"MetricsPrefix diverging from the check name is used instead",
			integration.Config{Name: "gearmand", Discovery: &integration.DiscoveryConfig{MetricsPrefix: "gearman"}},
			"gearman",
		},
		{
			"MetricsPrefix diverging from the check name and itself multi-segment: only the root is used",
			integration.Config{Name: "gearmand", Discovery: &integration.DiscoveryConfig{MetricsPrefix: "gearman.jobs"}},
			"gearman",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExpectedNamespaceRoot(tc.cfg))
		})
	}
}

func TestIsGenericIntegrationCheckName(t *testing.T) {
	assert.True(t, IsGenericIntegrationCheckName("openmetrics"))
	assert.True(t, IsGenericIntegrationCheckName("prometheus"))
	assert.False(t, IsGenericIntegrationCheckName("krakend"))
	assert.False(t, IsGenericIntegrationCheckName(""))
}

func TestGenericIntegrationNamespaceRoots(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want []string
	}{
		{
			"explicit namespace",
			"namespace: krakend.api\nopenmetrics_endpoint: http://1.2.3.4:9091/metrics",
			[]string{"krakend"},
		},
		{
			"no namespace, no metrics",
			"openmetrics_endpoint: http://1.2.3.4:9092/metrics",
			nil,
		},
		{
			"no namespace, plain-string metrics entries are pass-through, not renames",
			"metrics:\n  - envoy_cluster_http2_streams_active\n  - envoy_.*",
			nil,
		},
		{
			"no namespace, single-key map to string is a rename",
			"metrics:\n  - envoy_cluster_http2_streams_active: envoy.cluster.http2.streams_active",
			[]string{"envoy"},
		},
		{
			"no namespace, single-key map to nested map with name is a rename",
			"metrics:\n  - envoy_cluster_http2_streams_active:\n      name: envoy.cluster.http2.streams_active\n      type: rate",
			[]string{"envoy"},
		},
		{
			"no namespace, single-key map to nested map without name keeps the raw name, not a rename",
			"metrics:\n  - envoy_cluster_http2_streams_active:\n      type: rate",
			nil,
		},
		{
			"no namespace, extra_metrics handled the same as metrics",
			"extra_metrics:\n  - envoy_cluster_http2_streams_active: envoy.cluster.http2.streams_active",
			[]string{"envoy"},
		},
		{
			"namespace set: metrics renames are ignored, since namespace is prepended regardless",
			"namespace: myapp\nmetrics:\n  - envoy_cluster_http2_streams_active: envoy.cluster.http2.streams_active",
			[]string{"myapp"},
		},
		{
			"malformed yaml is skipped gracefully",
			"not: valid: yaml: [",
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := integration.Config{
				Name:      "openmetrics",
				Instances: []integration.Data{[]byte(tc.yaml)},
			}
			assert.Equal(t, tc.want, GenericIntegrationNamespaceRoots(cfg))
		})
	}
}
