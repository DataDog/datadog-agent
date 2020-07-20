// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/stretchr/testify/assert"
)

type metricsExpected struct {
	val  float64
	name string
	tags []string
}

func TestProcessMetrics(t *testing.T) {
	tests := []struct {
		name               string
		config             *KSMConfig
		metricsToProcess   map[string][]ksmstore.DDMetricsFam
		metricsToGet       []ksmstore.DDMetricsFam
		metricTransformers map[string]metricTransformerFunc
		expected           []metricsExpected
	}{
		{
			name:   "one metric family, default label mapper",
			config: &KSMConfig{LabelsMapper: defaultLabelsMapper},
			metricsToProcess: map[string][]ksmstore.DDMetricsFam{
				"kube_pod_container_status_running": {
					{
						Type: "*v1.Pod",
						Name: "kube_pod_container_status_running",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"container": "kube-state-metrics", "namespace": "default", "pod": "kube-state-metrics-b7fbc487d-4phhj", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
						},
					},
					{
						Type: "*v1.Pod",
						Name: "kube_pod_container_status_running",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"container": "hello", "namespace": "default", "pod": "hello-1509998340-k4f8q", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
						},
					},
				},
			},
			metricsToGet:       []ksmstore.DDMetricsFam{},
			metricTransformers: metricTransformers,
			expected: []metricsExpected{
				{
					name: "kubernetes_state.container.running",
					val:  1,
					tags: []string{"kube_container_name:kube-state-metrics", "kube_namespace:default", "pod_name:kube-state-metrics-b7fbc487d-4phhj", "uid:bec19172-8abf-11ea-8546-42010a80022c"},
				},
				{
					name: "kubernetes_state.container.running",
					val:  0,
					tags: []string{"kube_container_name:hello", "kube_namespace:default", "pod_name:hello-1509998340-k4f8q", "uid:05e99c5f-8a64-11ea-8546-42010a80022c"},
				},
			},
		},
		{
			name:   "host tag via label join, default label mapper, default label joins",
			config: &KSMConfig{LabelsMapper: defaultLabelsMapper, LabelJoins: defaultLabelJoins},
			metricsToProcess: map[string][]ksmstore.DDMetricsFam{
				"kube_pod_container_status_running": {
					{
						Type: "*v1.Pod",
						Name: "kube_pod_container_status_running",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"container": "kube-state-metrics", "namespace": "default", "pod": "kube-state-metrics-b7fbc487d-4phhj"},
								Val:    1,
							},
						},
					},
				},
			},
			metricsToGet: []ksmstore.DDMetricsFam{
				{
					Name:        "kube_pod_info",
					ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"created_by_kind": "ReplicaSet", "created_by_name": "kube-state-metrics-b7fbc487d", "host_ip": "192.168.99.100", "namespace": "default", "node": "minikube", "pod": "kube-state-metrics-b7fbc487d-4phhj", "pod_ip": "172.17.0.7"}}},
				},
			},
			metricTransformers: metricTransformers,
			expected: []metricsExpected{
				{
					name: "kubernetes_state.container.running",
					val:  1,
					tags: []string{"kube_container_name:kube-state-metrics", "kube_namespace:default", "pod_name:kube-state-metrics-b7fbc487d-4phhj", "host:minikube"},
				},
			},
		},
		{
			name:   "metadata metric, ignored",
			config: &KSMConfig{LabelsMapper: defaultLabelsMapper},
			metricsToProcess: map[string][]ksmstore.DDMetricsFam{
				"kube_pod_info": {
					{
						Type: "*v1.Pod",
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"created_by_kind": "ReplicaSet", "created_by_name": "kube-state-metrics-b7fbc487d", "host_ip": "192.168.99.100", "namespace": "default", "node": "minikube", "pod": "kube-state-metrics-b7fbc487d-4phhj", "pod_ip": "172.17.0.7"},
								Val:    1,
							},
						},
					},
				},
			},
			metricsToGet:       []ksmstore.DDMetricsFam{},
			metricTransformers: metricTransformers,
			expected:           []metricsExpected{},
		},
		{
			name:   "datadog standard tags via label join, default label mapper, default label joins",
			config: &KSMConfig{LabelsMapper: defaultLabelsMapper, LabelJoins: defaultLabelJoins},
			metricsToProcess: map[string][]ksmstore.DDMetricsFam{
				"kube_deployment_status_replicas": {
					{
						Type: "*v1.Deployment",
						Name: "kube_deployment_status_replicas",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"namespace": "default", "deployment": "redis"},
								Val:    1,
							},
						},
					},
				},
			},
			metricsToGet: []ksmstore.DDMetricsFam{
				{
					Name:        "kube_deployment_labels",
					ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"namespace": "default", "deployment": "redis", "label_tags_datadoghq_com_env": "dev", "label_tags_datadoghq_com_service": "redis", "label_tags_datadoghq_com_version": "v1"}}},
				},
			},
			metricTransformers: metricTransformers,
			expected: []metricsExpected{
				{
					name: "kubernetes_state.deployment.replicas",
					val:  1,
					tags: []string{"kube_namespace:default", "kube_deployment:redis", "env:dev", "service:redis", "version:v1"},
				},
			},
		},
		{
			name:   "honour metric transformers",
			config: &KSMConfig{LabelsMapper: defaultLabelsMapper},
			metricsToProcess: map[string][]ksmstore.DDMetricsFam{
				"kube_pod_status_phase": {
					{
						Type: "*v1.Pod",
						Name: "kube_pod_status_phase",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"namespace": "default", "pod": "redis-599d64fcb9-c654j", "phase": "Running"},
								Val:    1,
							},
						},
					},
				},
			},
			metricsToGet: []ksmstore.DDMetricsFam{},
			metricTransformers: map[string]metricTransformerFunc{
				"kube_pod_status_phase": func(s aggregator.Sender, n string, m ksmstore.DDMetric, t []string) {
					s.Gauge("kube_pod_status_phase_transformed", 1, "", []string{"transformed:tag"})
				},
			},
			expected: []metricsExpected{
				{
					name: "kube_pod_status_phase_transformed",
					val:  1,
					tags: []string{"transformed:tag"},
				},
			},
		},
	}
	for _, test := range tests {
		kubeStateMetricsSCheck := newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), test.config)
		mocked := mocksender.NewMockSender(kubeStateMetricsSCheck.ID())
		mocked.SetupAcceptAll()

		metricTransformers = test.metricTransformers
		kubeStateMetricsSCheck.processMetrics(mocked, test.metricsToProcess, test.metricsToGet)
		t.Run(test.name, func(t *testing.T) {
			for _, expectMetric := range test.expected {
				mocked.AssertMetric(t, "Gauge", expectMetric.name, expectMetric.val, "", expectMetric.tags)
			}
			if len(test.expected) == 0 {
				mocked.AssertNotCalled(t, "Gauge")
			} else {
				mocked.AssertNumberOfCalls(t, "Gauge", lenMetrics(test.metricsToProcess))
			}
		})
	}
}

func Test_isMatching(t *testing.T) {
	type args struct {
		config     *JoinsConfig
		destLabels map[string]string
		srcLabels  map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "match",
			args: args{
				config:     &JoinsConfig{LabelsToMatch: []string{"foo"}},
				destLabels: map[string]string{"foo": "bar", "baz": "bar"},
				srcLabels:  map[string]string{"foo": "bar"},
			},
			want: true,
		},
		{
			name: "no match",
			args: args{
				config:     &JoinsConfig{LabelsToMatch: []string{"foo"}},
				destLabels: map[string]string{"foo": "bar", "baz": "bar"},
				srcLabels:  map[string]string{"baz": "bar"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMatching(tt.args.config, tt.args.srcLabels, tt.args.destLabels); got != tt.want {
				t.Errorf("isMatching() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKSMCheck_joinLabels(t *testing.T) {
	type args struct {
		labels       map[string]string
		metricsToGet []ksmstore.DDMetricsFam
	}
	tests := []struct {
		name       string
		labelJoins map[string]*JoinsConfig
		args       args
		wantTags   []string
	}{
		{
			name: "join labels, multiple match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label", "bar_label"},
					LabelsToGet:   []string{"baz_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value"},
		},
		{
			name: "join labels, multiple get",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					LabelsToGet:   []string{"bar_label", "baz_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value"},
		},
		{
			name: "no label match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					LabelsToGet:   []string{"bar_label"},
				},
			},
			args: args{
				labels: map[string]string{"baz_label": "baz_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"baz_label:baz_value"},
		},
		{
			name: "no metric name match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					LabelsToGet:   []string{"bar_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "bar",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value"},
		},
		{
			name: "join labels, multiple metric match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label", "bar_label"},
					LabelsToGet:   []string{"baz_label"},
				},
				"bar": {
					LabelsToMatch: []string{"bar_label"},
					LabelsToGet:   []string{"baf_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
					{
						Name:        "bar",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"bar_label": "bar_value", "baf_label": "baf_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value", "baf_label:baf_value"},
		},
		{
			name: "join all labels",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					GetAllLabels:  true,
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeStateMetricsSCheck := newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), &KSMConfig{LabelJoins: tt.labelJoins})
			assert.ElementsMatch(t, tt.wantTags, kubeStateMetricsSCheck.joinLabels(tt.args.labels, tt.args.metricsToGet))
		})
	}
}

func TestKSMCheck_mergeLabelsMapper(t *testing.T) {
	tests := []struct {
		name     string
		config   *KSMConfig
		extra    map[string]string
		expected map[string]string
	}{
		{
			name:     "collision",
			config:   &KSMConfig{LabelsMapper: map[string]string{"foo": "bar", "baz": "baf"}},
			extra:    map[string]string{"foo": "tar", "tar": "foo"},
			expected: map[string]string{"foo": "bar", "baz": "baf", "tar": "foo"},
		},
		{
			name:     "no collision",
			config:   &KSMConfig{LabelsMapper: map[string]string{"foo": "bar", "baz": "baf"}},
			extra:    map[string]string{"tar": "foo"},
			expected: map[string]string{"foo": "bar", "baz": "baf", "tar": "foo"},
		},
		{
			name:     "empty LabelsMapper",
			config:   &KSMConfig{LabelsMapper: map[string]string{}},
			extra:    map[string]string{"tar": "foo"},
			expected: map[string]string{"tar": "foo"},
		},
		{
			name:     "empty extra",
			config:   &KSMConfig{LabelsMapper: map[string]string{"tar": "foo"}},
			extra:    map[string]string{},
			expected: map[string]string{"tar": "foo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &KSMCheck{instance: tt.config}
			k.mergeLabelsMapper(tt.extra)
			assert.True(t, reflect.DeepEqual(tt.expected, k.instance.LabelsMapper))
		})
	}
}

func TestMetadataMetricsRegex(t *testing.T) {
	metadataMetrics := []string{
		"kube_cronjob_info",
		"kube_job_info",
		"kube_pod_container_info",
		"kube_pod_info",
		"kube_service_info",
		"kube_persistentvolume_info",
		"kube_persistentvolumeclaim_info",
		"kube_deployment_labels",
		"kube_namespace_labels",
		"kube_node_labels",
		"kube_daemonset_labels",
		"kube_pod_labels",
		"kube_service_labels",
		"kube_statefulset_labels",
		"kube_verticalpodautoscaler_labels",
	}
	for _, m := range metadataMetrics {
		assert.True(t, metadataMetricsRegex.MatchString(m))
	}
}

func lenMetrics(metricsToProcess map[string][]ksmstore.DDMetricsFam) int {
	count := 0
	for _, metricFamily := range metricsToProcess {
		for _, metrics := range metricFamily {
			count += len(metrics.ListMetrics)
		}
	}
	return count
}
