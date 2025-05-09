// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/clock"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type rcCallbackFunc func(map[string]state.RawConfig, func(string, state.ApplyStatus))

type mockRCClient struct {
	subscribers map[string][]rcCallbackFunc
}

func (m *mockRCClient) SubscribeIgnoreExpiration(product string, callback func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	if m.subscribers == nil {
		m.subscribers = make(map[string][]rcCallbackFunc)
	}
	m.subscribers[product] = append(m.subscribers[product], callback)
}

func (m *mockRCClient) triggerUpdate(product string, update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	callbacks := m.subscribers[product]

	for _, callback := range callbacks {
		callback(update, applyStateCallback)
	}
}

func newMockConfigRetriever(t *testing.T, isLeader func() bool, store *store, clock clock.WithTicker) (*ConfigRetriever, *mockRCClient) {
	t.Helper()

	mockRCClient := &mockRCClient{}

	cr, err := NewConfigRetriever(context.Background(), clock, store, isLeader, mockRCClient)
	assert.NoError(t, err)

	return cr, mockRCClient
}

func buildRawConfig(t *testing.T, product string, version uint64, content []byte) state.RawConfig {
	t.Helper()

	return state.RawConfig{
		Metadata: state.Metadata{
			Product: product,
			Version: version,
		},
		Config: content,
	}
}
