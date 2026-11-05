// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package mock_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	demultiplexermock "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestNew(t *testing.T) {
	demux := demultiplexermock.New(t)
	sender, err := demux.GetDefaultSender()
	require.NoError(t, err)
	require.NotNil(t, sender)
	require.NotNil(t, demux.Serializer())

	replacement := &mocksender.MockSender{}
	demux.SetDefaultSender(replacement)
	sender, err = demux.GetDefaultSender()
	require.NoError(t, err)
	require.Same(t, replacement, sender)

	demux.Stop()
	demux.Stop()
}

func TestNewFakeSamplerMock(t *testing.T) {
	demux := demultiplexermock.NewFakeSamplerMock(t)
	require.NotNil(t, demux.GetAgentDemultiplexer())

	demux.Stop()
	demux.Stop()
}
