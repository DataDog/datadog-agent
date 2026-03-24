// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package processimpl

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	processStatus "github.com/DataDog/datadog-agent/pkg/process/util/status"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetStatusDetails(t *testing.T) {
	env.SetFeatures(t)

	cfg := config.NewMock(t)
	cfg.SetWithoutSource("hostname", "test-host")

	impl := &remoteagentImpl{
		cfg:      cfg,
		hostname: hostnameimpl.NewHostnameService(),
	}

	resp, err := impl.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp.MainSection)
	require.Contains(t, resp.MainSection.Fields, "status")

	var st processStatus.Status
	require.NoError(t, json.Unmarshal([]byte(resp.MainSection.Fields["status"]), &st))

	assert.NotZero(t, st.Date)

	// core section populated
	assert.NotEmpty(t, st.Core.AgentVersion)
	assert.NotEmpty(t, st.Core.GoVersion)
	assert.NotEmpty(t, st.Core.Arch)

	// Pid comes from os.Getpid() directly not from expvar
	assert.Equal(t, os.Getpid(), st.Expvars.ExpvarsMap.Pid)

	// nonzero mem stats
	assert.Greater(t, st.Expvars.ExpvarsMap.MemStats.Alloc, uint64(0))
}
