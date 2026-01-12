// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build unix

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
		{GlobalParams{ConfFilePath: "/a/b/c"}, "/a/b/c"},
		{GlobalParams{ConfFilePath: "/a/b/c/system-probe.yaml"}, "/a/b/c"},
		{GlobalParams{ConfFilePath: "/a/b/c", datadogConfFilePath: "/x/y"}, "/x/y"},
		{GlobalParams{ConfFilePath: "/a/b/c", datadogConfFilePath: "/x/y/datadog.yaml"}, "/x/y/datadog.yaml"},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, c.params.DatadogConfFilePath(), "%+v", c.params)
	}
}
