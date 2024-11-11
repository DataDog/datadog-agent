// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package slis

import (
	"errors"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
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

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestProvider_Provide(t *testing.T) {
	slisEndpoint := "http://10.8.0.1:10255/metrics/slis"

	type metrics struct {
		name  string
		value float64
		tags  []string
	}

	expectedMetrics := []metrics{
		{
			name:  common.KubeletMetricsPrefix + "slis.kubernetes_healthcheck",
			value: 1,
			tags:  []string{"sli_name:log"},
		},
		{
			name:  common.KubeletMetricsPrefix + "slis.kubernetes_healthcheck",
			value: 1,
			tags:  []string{"sli_name:ping"},
		},
		{
			name:  common.KubeletMetricsPrefix + "slis.kubernetes_healthcheck",
			value: 1,
			tags:  []string{"sli_name:syncloop"},
		},
		{
			name:  common.KubeletMetricsPrefix + "slis.kubernetes_healthchecks_total",
			value: 14319,
			tags:  []string{"sli_name:log", "status:success"},
		},
		{
			name:  common.KubeletMetricsPrefix + "slis.kubernetes_healthchecks_total",
			value: 14321,
			tags:  []string{"sli_name:ping", "status:success"},
		},
		{
			name:  common.KubeletMetricsPrefix + "slis.kubernetes_healthchecks_total",
			value: 14319,
			tags:  []string{"sli_name:syncloop", "status:success"},
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
		name         string
		podsFile     string
		slisEndpoint *string
		response     response
		want         want
	}{
		{
			name:         "slis exist metrics are reported",
			podsFile:     "../../testdata/pods.json",
			slisEndpoint: &slisEndpoint,
			response: response{
				filename: "../../testdata/slis.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: expectedMetrics,
			},
		},
		{
			name:         "endpoint 404 returns no error",
			podsFile:     "../../testdata/pods.json",
			slisEndpoint: &slisEndpoint,
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
			name:         "endpoint error returns error",
			podsFile:     "../../testdata/pods.json",
			slisEndpoint: &slisEndpoint,
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
			name:         "no pod metadata no metrics reported",
			podsFile:     "",
			slisEndpoint: &slisEndpoint,
			response: response{
				filename: "../../testdata/slis.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: nil,
				err:     nil,
			},
		},
		{
			name:         "no slis endpoint supplied default used metrics reported",
			podsFile:     "../../testdata/pods.json",
			slisEndpoint: nil,
			response: response{
				filename: "../../testdata/slis.txt",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: expectedMetrics,
				err:     nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
			mockSender.SetupAcceptAll()

			fakeTagger := taggerimpl.SetupFakeTagger(t)

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
			kubeletMock.MockReplies["/metrics/slis"] = &mock.HTTPReplyMock{
				Data:         content,
				ResponseCode: tt.response.code,
				Error:        tt.response.err,
			}

			config := &common.KubeletConfig{
				SlisMetricsEndpoint: tt.slisEndpoint,
			}

			p, err := NewProvider(
				&containers.Filter{
					Enabled:         true,
					NameExcludeList: []*regexp.Regexp{regexp.MustCompile("agent-excluded")},
				},
				config,
				store,
			)
			assert.NoError(t, err)

			err = p.Provide(kubeletMock, mockSender)
			if !reflect.DeepEqual(err, tt.want.err) {
				t.Errorf("Collect() error = %v, wantErr %v", err, tt.want.err)
				return
			}

			typeTag := []string{"type:healthz"}
			for _, metric := range tt.want.metrics {
				if metric.name == common.KubeletMetricsPrefix+"slis.kubernetes_healthcheck" {
					mockSender.AssertMetric(t, "Gauge", metric.name, metric.value, "", metric.tags)
					mockSender.AssertMetricNotTaggedWith(t, "Gauge", metric.name, typeTag)
				} else {
					mockSender.AssertMetric(t, "Count", metric.name, metric.value, "", metric.tags)
					mockSender.AssertMetricNotTaggedWith(t, "Count", metric.name, typeTag)
				}
			}
		})
	}
}

func TestProvider_DisableProvider(t *testing.T) {
	slisEndpoint := "http://10.8.0.1:10255/metrics/slis"

	var err error

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
	mockSender.SetupAcceptAll()

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	err = commontesting.StorePopulatedFromFile(store, "../../testdata/pods.json", common.NewPodUtils(fakeTagger))
	if err != nil {
		t.Errorf("unable to populate store from file at: %s, err: %v", "../../testdata/pods.json", err)
	}

	kubeletMock := mock.NewKubeletMock()
	var content []byte
	kubeletMock.MockReplies["/metrics/slis"] = &mock.HTTPReplyMock{
		Data:         content,
		ResponseCode: 404,
		Error:        nil,
	}

	config := &common.KubeletConfig{
		SlisMetricsEndpoint: &slisEndpoint,
	}

	p, err := NewProvider(
		&containers.Filter{
			Enabled:         true,
			NameExcludeList: []*regexp.Regexp{regexp.MustCompile("agent-excluded")},
		},
		config,
		store,
	)
	assert.NoError(t, err)

	err = p.Provide(kubeletMock, mockSender)
	assert.Truef(t, p.ScraperConfig.IsDisabled, "provider should be disabled")
	if !reflect.DeepEqual(err, nil) {
		t.Errorf("Collect() error = %v, wantErr %v", err, nil)
		return
	}
}
