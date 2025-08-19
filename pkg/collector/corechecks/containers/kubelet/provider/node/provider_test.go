// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package node

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
)

func TestProvider_Provide(t *testing.T) {
	type response struct {
		filename string
		code     int
		err      error
	}
	type metrics struct {
		name  string
		value float64
		tags  []string
	}
	type want struct {
		metrics []metrics
		err     error
	}
	tests := []struct {
		name     string
		response response
		want     want
	}{
		{
			name: "endpoint returns spec",
			response: response{
				filename: "../../testdata/node_spec.json",
				code:     200,
				err:      nil,
			},
			want: want{
				metrics: []metrics{
					{
						name:  common.KubeletMetricsPrefix + "cpu.capacity",
						value: 1,
						tags:  []string{"instance_tag:something"},
					},
					{
						name:  common.KubeletMetricsPrefix + "memory.capacity",
						value: 3885424640,
						tags:  []string{"instance_tag:something"},
					},
				},
				err: nil,
			},
		},
		{
			name: "endpoint does not exist returns no error",
			response: response{
				code: 404,
				err:  errors.New("page not found"),
			},
			want: want{
				metrics: nil,
				err:     nil,
			},
		},
		{
			name: "endpoint returns non 404 returns error",
			response: response{
				code: 401,
				err:  errors.New("unauthorized"),
			},
			want: want{
				metrics: nil,
				err:     errors.New("unauthorized"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
			mockSender.SetupAcceptAll()

			kubeletMock := mock.NewKubeletMock()
			var content []byte
			var err error
			if tt.response.filename != "" {
				content, err = os.ReadFile(tt.response.filename)
				if err != nil {
					t.Errorf("unable to read test file at: %s, err: %v", tt.response.filename, err)
				}
			}
			kubeletMock.MockReplies["/spec/"] = &mock.HTTPReplyMock{
				Data:         content,
				ResponseCode: tt.response.code,
				Error:        tt.response.err,
			}

			config := &common.KubeletConfig{
				OpenmetricsInstance: types.OpenmetricsInstance{
					Tags: []string{"instance_tag:something"},
				},
			}

			p := &Provider{
				config: config,
			}
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
