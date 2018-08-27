// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver
package custommetrics

import "testing"

type mockProcessor struct {
	sink chan PodMetricValue
}

func newMockProcessor() *mockProcessor {
	return &mockProcessor{
		sink: make(chan PodMetricValue, 1),
	}
}

func (m *mockProcessor) Start() {}

func (m *mockProcessor) Process(podMetric PodMetricValue) {
	m.sink <- podMetric
}

func (m *mockProcessor) Stop() error { return nil }

func TestBufferedProcessor(t *testing.T) {
	// TODO
}
