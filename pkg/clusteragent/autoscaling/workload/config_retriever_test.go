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

	rcmock "github.com/DataDog/datadog-agent/pkg/config/remote/client/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func newMockConfigRetriever(t *testing.T, isLeader func() bool, store *store, clock clock.WithTicker) (*ConfigRetriever, *rcmock.Client) {
	t.Helper()

	mockRCClient := rcmock.New()

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
