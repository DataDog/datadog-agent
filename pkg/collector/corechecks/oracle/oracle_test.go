// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestBasic(t *testing.T) {
	chk := Check{}

	// language=yaml
	rawInstanceConfig := []byte(`
server: localhost,1521
username: system
password: password
service_name: XE
`)

	err := chk.Configure(integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	assert.NoError(t, err)
}
