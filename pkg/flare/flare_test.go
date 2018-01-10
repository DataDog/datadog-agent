// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestMkURL(t *testing.T) {
	common.SetupConfig("./test")
	config.Datadog.Set("dd_url", "https://example.com")
	config.Datadog.Set("api_key", "123456")
	assert.Equal(t, "https://example.com/support/flare/999?api_key=123456", mkURL("999"))
	assert.Equal(t, "https://example.com/support/flare?api_key=123456", mkURL(""))
}
