// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostnameimpl

import (
	"context"
	"testing"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	comp, _ := hostnamemock.New("test-hostname")
	name, err := comp.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-hostname", name)
}

func TestGetWithProvider(t *testing.T) {
	comp, _ := hostnamemock.New("test-hostname2")
	data, err := comp.GetWithProvider(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-hostname2", data.Hostname)
	assert.Equal(t, "mockService", data.Provider)
}
