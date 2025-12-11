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
		{GlobalParams{ConfFilePath: "C:\\a\\b\\c"}, "C:\\a\\b\\c"},
		{GlobalParams{ConfFilePath: "C:\\a\\b\\c\\system-probe.yaml"}, "C:\\a\\b\\c"},
		{GlobalParams{ConfFilePath: "C:\\a\\b\\c", datadogConfFilePath: "C:\\x\\y"}, "C:\\x\\y"},
		{GlobalParams{ConfFilePath: "C:\\a\\b\\c", datadogConfFilePath: "C:\\x\\y\\datadog.yaml"}, "C:\\x\\y\\datadog.yaml"},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, c.params.DatadogConfFilePath(), "%+v", c.params)
	}
}
