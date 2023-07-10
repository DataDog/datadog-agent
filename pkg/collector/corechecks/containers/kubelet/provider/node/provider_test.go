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

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
)

func TestProvider_Collect(t *testing.T) {
	type response struct {
		filename string
		code     int
		err      error
	}
	type want struct {
		collected interface{}
		err       error
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
				collected: &nodeSpec{
					NumCores:       1,
					MemoryCapacity: 3885424640,
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
				collected: nil,
				err:       nil,
			},
		},
		{
			name: "endpoint returns non 404 returns error",
			response: response{
				code: 401,
				err:  errors.New("unauthorized"),
			},
			want: want{
				collected: nil,
				err:       errors.New("unauthorized"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			config := &common.KubeletConfig{Tags: []string{"instance_tag:something"}}

			p := &Provider{
				config: config,
			}
			got, err := p.Collect(kubeletMock)
			if !reflect.DeepEqual(err, tt.want.err) {
				t.Errorf("Collect() error = %v, wantErr %v", err, tt.want.err)
				return
			}
			if !reflect.DeepEqual(got, tt.want.collected) {
				t.Errorf("Collect() got = %v, want %v", got, tt.want.collected)
			}
		})
	}
}

func TestProvider_Transform(t *testing.T) {
	type args struct {
		spec *nodeSpec
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "spec is nil reports nothing",
		},
		{
			name: "spec is not nil reports metrics",
			args: args{spec: &nodeSpec{
				NumCores:       1,
				MemoryCapacity: 3885424640,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(check.ID(t.Name()))
			mockSender.SetupAcceptAll()

			config := &common.KubeletConfig{Tags: []string{"instance_tag:something"}}

			p := &Provider{
				config: config,
			}
			if err := p.Transform(tt.args.spec, mockSender); (err != nil) != tt.wantErr {
				t.Errorf("Transform() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.args.spec == nil {
				mockSender.AssertNumberOfCalls(t, "Gauge", 0)
			} else {
				mockSender.AssertNumberOfCalls(t, "Gauge", 2)

				mockSender.AssertMetric(t, "Gauge", common.KubeletMetricsPrefix+"cpu.capacity", tt.args.spec.NumCores, "", []string{"instance_tag:something"})
				mockSender.AssertMetric(t, "Gauge", common.KubeletMetricsPrefix+"memory.capacity", tt.args.spec.MemoryCapacity, "", []string{"instance_tag:something"})
			}
		})
	}
}
