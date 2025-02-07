// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

func TestStatusOut(t *testing.T) {
	tests := []struct {
		name       string
		assertFunc func(t *testing.T, headerProvider status.Provider)
	}{
		{"JSON", func(t *testing.T, headerProvider status.Provider) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)

			assert.NotEmpty(t, stats)
		}},
		{"Text", func(t *testing.T, headerProvider status.Provider) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T, headerProvider status.Provider) {
			b := new(bytes.Buffer)
			err := headerProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provides := NewComponent(Requires{
				Config:    config.NewMock(t),
				Authtoken: authtoken.Component(&fetchonlyimpl.MockFetchOnly{}),
			})
			headerProvider := provides.StatusProvider.Provider
			test.assertFunc(t, headerProvider)
		})
	}
}
