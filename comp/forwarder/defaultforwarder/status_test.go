// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestJSON(t *testing.T) {
	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
	))

	provider := statusProvider{
		config: config,
	}

	status := make(map[string]interface{})
	provider.JSON(false, status)

	jsonString, err := json.Marshal(status)
	assert.NoError(t, err)

	stats := status["forwarderStats"].(map[string]interface{})

	assert.Nil(t, stats["forwarder_storage_max_size_in_bytes"])

	assert.NotEqual(t, "", string(jsonString))
}

func TestJSONWith_forwarder_storage_max_size_in_bytes(t *testing.T) {

	overrides := map[string]interface{}{
		"forwarder_storage_max_size_in_bytes": 67,
	}

	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	provider := statusProvider{
		config: config,
	}

	status := make(map[string]interface{})
	provider.JSON(false, status)

	stats := status["forwarderStats"].(map[string]interface{})

	assert.Equal(t, "67", stats["forwarder_storage_max_size_in_bytes"])
}

func TestText(t *testing.T) {
	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
	))

	provider := statusProvider{
		config: config,
	}

	b := new(bytes.Buffer)
	provider.Text(false, b)

	assert.NotEqual(t, "", b.String())
}

func TestHTML(t *testing.T) {
	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
	))

	provider := statusProvider{
		config: config,
	}

	b := new(bytes.Buffer)
	provider.HTML(false, b)

	assert.NotEqual(t, "", b.String())
}
