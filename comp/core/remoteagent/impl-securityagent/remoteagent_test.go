// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package securityagentimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// statusMock is a minimal status.Component implementation for testing.
type statusMock struct {
	statusJSON []byte
	err        error
}

func (m *statusMock) GetStatus(_ string, _ bool, _ ...string) ([]byte, error) {
	return m.statusJSON, m.err
}

func (m *statusMock) GetSections() []string {
	return nil
}

func (m *statusMock) GetStatusBySections(_ []string, _ string, _ bool) ([]byte, error) {
	return m.statusJSON, m.err
}

var _ status.Component = (*statusMock)(nil)

func TestGetStatusDetails_IncludesExpvarAndStatus(t *testing.T) {
	statusJSON := []byte(`{"agent":"security-agent","status":"running"}`)
	impl := &remoteagentImpl{
		cfg:        config.NewMock(t),
		statusComp: &statusMock{statusJSON: statusJSON},
	}

	resp, err := impl.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp.MainSection)
	assert.Equal(t, string(statusJSON), resp.MainSection.Fields["status"])
}
