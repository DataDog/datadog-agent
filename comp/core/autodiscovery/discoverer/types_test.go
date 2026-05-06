// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestResultZeroValueIsEmpty(t *testing.T) {
	var r Result
	assert.Empty(t, r.Configs)
}

func TestResultPreservesConfigs(t *testing.T) {
	cfgs := []integration.Config{{Name: "krakend"}, {Name: "krakend"}}
	r := Result{Configs: cfgs}
	assert.Len(t, r.Configs, 2)
	assert.Equal(t, "krakend", r.Configs[0].Name)
}
