// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"

	kubeletv1alpha1 "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

// HTTPReplyMock represents a fake HTTP reply
type HTTPReplyMock struct {
	Data         []byte
	ResponseCode int
	Error        error
}

// KubeletMock is a fake Kubelet implementation
type KubeletMock struct {
	kubelet.KubeUtil
	MockReplies map[string]*HTTPReplyMock
}

// NewKubeletMock returns a mock instance
func NewKubeletMock() *KubeletMock {
	return &KubeletMock{
		MockReplies: make(map[string]*HTTPReplyMock),
	}
}

// QueryKubelet overrides base implementation using HTTPReplyMock
func (km *KubeletMock) QueryKubelet(ctx context.Context, path string) ([]byte, int, error) {
	reply := km.MockReplies[path]
	if reply != nil {
		return reply.Data, reply.ResponseCode, reply.Error
	}

	return nil, http.StatusNotFound, nil
}

// GetLocalStatsSummary is a mock method
func (km *KubeletMock) GetLocalStatsSummary(ctx context.Context) (*kubeletv1alpha1.Summary, error) {
	data, rc, err := km.QueryKubelet(ctx, "/stats/summary")
	if err != nil {
		return nil, err
	}

	if rc != http.StatusOK {
		return nil, fmt.Errorf("Unable to fetch stats summary from Kubelet, rc: %d", rc)
	}

	statsSummary := &kubeletv1alpha1.Summary{}
	if err := json.Unmarshal(data, statsSummary); err != nil {
		return nil, err
	}

	return statsSummary, nil
}
