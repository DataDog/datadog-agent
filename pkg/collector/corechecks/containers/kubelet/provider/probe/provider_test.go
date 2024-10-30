// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package probe

import (
	"context"
	"errors"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	commontesting "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common/testing"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
)

var (
	probeTags = map[string][]string{
		"container_id://2c3f5608164033a850c9acbbfdb7fffa6ce1f68feedb1b8dad99777373c35b16": {
			"kube_namespace:kube-system",
			"pod_name:kube-dns-c598bd956-wgf4n",
			"kube_container_name:sidecar",
		},
		"container_id://b13f7638c80c98946900bdeabec06be564d203330f5bb706a40e6fa7466a674d": {
			"kube_namespace:kube-system",
			"pod_name:kube-dns-c598bd956-wgf4n",
			"kube_container_name:kubedns",
		},
		"container_id://3102f0d9499c5cd0875225208e3d048e3a9d829f5cdd74758b2d79a429a579fa": {
			"kube_namespace:kube-system",
			"pod_name:fluentbit-gke-45gvm",
			"kube_container_name:fluentbit-gke",
		},
		"container_id://efa5b57cc110de6d2ca3b4a0e12c0a378090530e5e2d0ba0882dffe9c5846067": {
			"kube_namespace:kube-system",
			"pod_name:fluentbit-gke-45gvm",
			"kube_container_name:fluentbit",
		},
		"container_id://0d8eea0b23688a4c3fbc29828b455734b323d6aac085c88f8f112e296cd5b521": {
			"kube_namespace:kube-system",
			"pod_name:kube-dns-c598bd956-wgf4n",
			"kube_container_name:dnsmasq",
		},
		"container_id://1669a6277ebb44aedd2790ba8bce83d21899ba85b1afde4330caf4a4161eee26": {
			"kube_namespace:kube-system",
			"pod_name:calico-node-9qkw7",
			"kube_container_name:calico-node",
		},
		"container_id://c81dfc25dd24b538a880bfd0f807ba9ec1ff4541e8b8eb49a8d1afcdecc5ef59": {
			"kube_namespace:default",
			"pod_name:datadog-t9f28",
			"kube_container_name:agent",
		},
	}
)

func TestProvider_Provide(t *testing.T) {
	probesEndpoint := "http://10.8.0.1:10255/metrics/probes"
	probesEndpointDisabled := ""

	type metrics struct {
		name  string
		value float64
		tags  []string
	}

	expectedMetrics := []metrics{
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.success.total",
			value: 3,
			tags:  []string{"instance_tag:something", "kube_namespace:default", "pod_name:datadog-t9f28", "kube_container_name:agent"},
		},
		{
			name:  common.KubeletMetricsPrefix + "readiness_probe.success.total",
			value: 3,
			tags:  []string{"instance_tag:something", "kube_namespace:default", "pod_name:datadog-t9f28", "kube_container_name:agent"},
		},
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.success.total",
			value: 281049,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:fluentbit-gke-45gvm", "kube_container_name:fluentbit"},
		},
		/* Excluded container is not expected, see containers.Filter in the test
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.success.total",
			value: 281049,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:fluentbit-gke-45gvm", "kube_container_name:fluentbit-gke"},
		},
		*/
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.success.total",
			value: 1686298,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:kube-dns-c598bd956-wgf4n", "kube_container_name:kubedns"},
		},
		{
			name:  common.KubeletMetricsPrefix + "readiness_probe.success.total",
			value: 1686303,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:kube-dns-c598bd956-wgf4n", "kube_container_name:kubedns"},
		},
		{
			name:  common.KubeletMetricsPrefix + "startup_probe.success.total",
			value: 70,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:kube-dns-c598bd956-wgf4n", "kube_container_name:kubedns"},
		},
		{
			name:  common.KubeletMetricsPrefix + "startup_probe.failure.total",
			value: 70,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:kube-dns-c598bd956-wgf4n", "kube_container_name:kubedns"},
		},
		{
			name:  common.KubeletMetricsPrefix + "readiness_probe.failure.total",
			value: 180,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:calico-node-9qkw7", "kube_container_name:calico-node"},
		},
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.failure.total",
			value: 100,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:calico-node-9qkw7", "kube_container_name:calico-node"},
		},
		{
			name:  common.KubeletMetricsPrefix + "readiness_probe.success.total",
			value: 1686127,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:calico-node-9qkw7", "kube_container_name:calico-node"},
		},
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.success.total",
			value: 1686306,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:calico-node-9qkw7", "kube_container_name:calico-node"},
		},
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.success.total",
			value: 1686298,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:kube-dns-c598bd956-wgf4n", "kube_container_name:sidecar"},
		},
		{
			name:  common.KubeletMetricsPrefix + "liveness_probe.success.total",
			value: 1686298,
			tags:  []string{"instance_tag:something", "kube_namespace:kube-system", "pod_name:kube-dns-c598bd956-wgf4n", "kube_container_name:dnsmasq"},
		},
	}

	type response struct {
		filename string
		code     int
		err      error
	}
	type want struct {
		metrics []metrics
		err     error
	}
	tests := []struct {
		name           string
		podsFile       string
		probesEndpoint *string
		response       response
		want           want
	}{
		{
			name:           "probes exist metrics are reported",
			podsFile:       "../../testdata/pod_list_probes.json",
			probesEndpoint: &probesEndpoint,
			response: response{
				filename: "../../testdata/probes.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: expectedMetrics,
			},
		},
		{
			name:           "endpoint 404 returns no error",
			podsFile:       "../../testdata/pod_list_probes.json",
			probesEndpoint: &probesEndpoint,
			response: response{
				filename: "",
				code:     404,
				err:      nil,
			},
			want: want{
				metrics: nil,
				err:     nil,
			},
		},
		{
			name:           "endpoint error returns error",
			podsFile:       "../../testdata/pod_list_probes.json",
			probesEndpoint: &probesEndpoint,
			response: response{
				filename: "",
				code:     0,
				err:      errors.New("some error happened"),
			},
			want: want{
				metrics: nil,
				err:     errors.New("some error happened"),
			},
		},
		{
			name:           "no pod metadata no metrics reported",
			podsFile:       "",
			probesEndpoint: &probesEndpoint,
			response: response{
				filename: "../../testdata/probes.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: nil,
				err:     nil,
			},
		},
		{
			name:           "no probe endpoint supplied default used metrics reported",
			podsFile:       "../../testdata/pod_list_probes.json",
			probesEndpoint: nil,
			response: response{
				filename: "../../testdata/probes.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: expectedMetrics,
				err:     nil,
			},
		},
		{
			name:           "empty string probe endpoint supplied no metrics reported",
			podsFile:       "../../testdata/pod_list_probes.json",
			probesEndpoint: &probesEndpointDisabled,
			response: response{
				filename: "../../testdata/probes.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: nil,
				err:     nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
			mockSender.SetupAcceptAll()

			fakeTagger := taggerimpl.SetupFakeTagger(t)

			for entity, tags := range probeTags {
				prefix, id, _ := taggercommon.ExtractPrefixAndID(entity)
				entityID := taggertypes.NewEntityID(prefix, id)
				fakeTagger.SetTags(entityID, "foo", tags, nil, nil, nil)
			}

			err = commontesting.StorePopulatedFromFile(store, tt.podsFile, common.NewPodUtils(fakeTagger))
			if err != nil {
				t.Errorf("unable to populate store from file at: %s, err: %v", tt.podsFile, err)
			}

			kubeletMock := mock.NewKubeletMock()
			var content []byte
			if tt.response.filename != "" {
				content, err = os.ReadFile(tt.response.filename)
				if err != nil {
					t.Errorf("unable to read test file at: %s, err: %v", tt.response.filename, err)
				}
			}
			kubeletMock.MockReplies["/metrics/probes"] = &mock.HTTPReplyMock{
				Data:         content,
				ResponseCode: tt.response.code,
				Error:        tt.response.err,
			}

			config := &common.KubeletConfig{
				OpenmetricsInstance: types.OpenmetricsInstance{
					Tags: []string{"instance_tag:something"},
				},
				ProbesMetricsEndpoint: tt.probesEndpoint,
			}

			p, err := NewProvider(
				&containers.Filter{
					Enabled:         true,
					NameExcludeList: []*regexp.Regexp{regexp.MustCompile("fluentbit-gke")},
				},
				config,
				store,
				fakeTagger,
			)
			assert.NoError(t, err)

			err = p.Provide(kubeletMock, mockSender)
			if !reflect.DeepEqual(err, tt.want.err) {
				t.Errorf("Collect() error = %v, wantErr %v", err, tt.want.err)
				return
			}
			mockSender.AssertNumberOfCalls(t, "Gauge", len(tt.want.metrics))
			for _, metric := range tt.want.metrics {
				mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
			}
		})
	}
}
