// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatadogConfPath(t *testing.T) {
	cases := []struct {
		params   GlobalParams
		expected string
	}{
		{GlobalParams{}, ""},
		{GlobalParams{ConfFilePath: "C:\\some\\other\\dir"}, "C:\\some\\other\\dir"},
		{GlobalParams{ConfFilePath: "C:\\some\\other\\dir\\system-probe.yaml"}, "C:\\some\\other\\dir"},
		{GlobalParams{ConfFilePath: "C:\\some\\other\\dir", datadogConfFilePath: "C:\\another\\dir"}, "C:\\another\\dir"},
		{GlobalParams{ConfFilePath: "C:\\some\\other\\dir", datadogConfFilePath: "C:\\another\\dir\\datadog.yaml"}, "C:\\another\\dir\\datadog.yaml"},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, c.params.DatadogConfFilePath(), "%+v", c.params)
	}
}
