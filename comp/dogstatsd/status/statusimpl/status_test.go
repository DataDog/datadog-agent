// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"

	// We need to include this to make sure the Dogstatsd expvars are initialized
	_ "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

func TestStatusDisabledWhenADPEnabled(t *testing.T) {
	config := configmock.New(t)
	config.Set("data_plane.enabled", true, configmodel.SourceAgentRuntime)
	config.Set("data_plane.dogstatsd.enabled", true, configmodel.SourceAgentRuntime)

	deps := dependencies{
		Config: config,
	}
	provides := newStatusProvider(deps)

	assert.Nil(t, provides.Status.Provider)
}

func TestStatusOutputPresent(t *testing.T) {
	deps := dependencies{
		Config: configmock.New(t),
	}
	provides := newStatusProvider(deps)

	statusProvider := provides.Status.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			err := statusProvider.JSON(false, stats)
			assert.NoError(t, err)
			assert.NotEmpty(t, stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := statusProvider.Text(false, b)
			assert.NoError(t, err)
			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := statusProvider.HTML(false, b)
			assert.NoError(t, err)
			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
