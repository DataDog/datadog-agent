// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package health

import (
	"errors"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
)

func TestProvider_Provide(t *testing.T) {
	type response struct {
		content []byte
		code    int
		err     error
	}
	type checkinfo struct {
		CheckName string
		status    servicecheck.ServiceCheckStatus
		msg       string
	}
	type want struct {
		servicechecks []checkinfo
		err           error
	}
	tests := []struct {
		name     string
		response response
		want     want
	}{
		{
			name: "endpoint returns returns ok health status without error code",
			response: response{
				code:    200,
				content: []byte("[+]ping ok\n[+]log ok\nhealthz check passed\n"),
				err:     nil,
			},
			want: want{
				servicechecks: []checkinfo{
					{CheckName: "kubernetes.kubelet.check.ping",
						status: servicecheck.ServiceCheckOK},
					{CheckName: "kubernetes.kubelet.check.log",
						status: servicecheck.ServiceCheckOK},
					{CheckName: "kubernetes.kubelet.check",
						status: servicecheck.ServiceCheckOK},
				},
				err: nil,
			},
		},
		{
			name: "endpoint returns returns bad health status",
			response: response{
				code:    200,
				content: []byte("[-]ping failed\n[+]log ok\nhealthz check failed\n"),
				err:     nil,
			},
			want: want{
				servicechecks: []checkinfo{
					{CheckName: "kubernetes.kubelet.check.ping",
						status: servicecheck.ServiceCheckCritical},
					{CheckName: "kubernetes.kubelet.check.log",
						status: servicecheck.ServiceCheckOK},
					{CheckName: "kubernetes.kubelet.check",
						status: servicecheck.ServiceCheckCritical,
						msg:    "Kubelet health check failed, http response code = 200"},
				},
				err: nil,
			},
		},
		{
			name: "endpoint returns returns error with error code",
			response: response{
				code: 401,
				err:  errors.New("unauthorized"),
			},
			want: want{
				servicechecks: nil,
				err:           errors.New("unauthorized"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
			mockSender.SetupAcceptAll()

			kubeletMock := mock.NewKubeletMock()
			var err error
			kubeletMock.MockReplies["/healthz?verbose"] = &mock.HTTPReplyMock{
				Data:         tt.response.content,
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
			if err == nil {
				mockSender.AssertNumberOfCalls(t, "ServiceCheck", len(tt.want.servicechecks))
				tags := []string{"instance_tag:something"}
				for _, servicecheck := range tt.want.servicechecks {
					mockSender.AssertServiceCheck(t,
						servicecheck.CheckName,
						servicecheck.status,
						"",
						tags,
						servicecheck.msg)
				}
			}
		})
	}
}
