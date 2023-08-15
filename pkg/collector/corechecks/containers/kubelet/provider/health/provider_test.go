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

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"

)

func createServiceCheck(checkName string, status servicecheck.ServiceCheckStatus) *servicecheck.ServiceCheck {
	return &servicecheck.ServiceCheck{
		CheckName: common.KubeletMetricsPrefix + "kubelet.check"  + checkName,
		Host:      "",
		Ts:        1,
		Status:    status,
		Message:   "",
		Tags:      []string{"instance_tag:something"}}
}

func TestProvider_Provide(t *testing.T) {
	type response struct {
		content  []byte
		code     int
		err      error
	}
	type want struct {
		servicechecks []*servicecheck.ServiceCheck 
		err     error
	}
	tests := []struct {
		name     string
		response response
		want     want
	}{
		{
			name: "endpoint returns returns ok health status without error code",
			response: response{
				code: 200,
				content: []byte("[+]ping ok\nhealthz check passed\n"),
				err:  nil,
			},
			want: want{
				servicechecks: []*servicecheck.ServiceCheck{
					createServiceCheck(".ping", servicecheck.ServiceCheckOK), 
					createServiceCheck("", servicecheck.ServiceCheckOK)},
				err:  nil,
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
				err:  errors.New("unauthorized"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
			mockSender.SetupAcceptAll()

			kubeletMock := mock.NewKubeletMock()
			var err error
			kubeletMock.MockReplies["/healthz"] = &mock.HTTPReplyMock{
				Data:         tt.response.content,
				ResponseCode: tt.response.code,
				Error:        tt.response.err,
			}

			config := &common.KubeletConfig{Tags: []string{"instance_tag:something"}}

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
			}
		})
	}
}
