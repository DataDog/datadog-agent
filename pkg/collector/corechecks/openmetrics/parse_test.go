// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePrometheusSampleLineStrictValidation(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "invalid metric name", line: "1metric 1"},
		{name: "invalid label name", line: "metric{1label=\"value\"} 1"},
		{name: "invalid label set", line: "metric{label} 1"},
		{name: "duplicate label", line: "metric{label=\"a\",label=\"b\"} 1"},
		{name: "invalid label escape", line: `metric{label="bad\qescape"} 1`},
		{name: "invalid utf8 label value", line: "metric{label=\"\xff\"} 1"},
		{name: "hex float", line: "metric 0x1p+2"},
		{name: "underscore float", line: "metric 1_000"},
		{name: "inline comment", line: "metric 1 # comment"},
		{name: "trailing timestamp data", line: "metric 1 123 extra"},
		{name: "missing value", line: "metric_without_value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parsePrometheusSampleLine([]byte(test.line), false)
			require.Error(t, err)
		})
	}
}

func TestParsePrometheusTypeLineStrictValidation(t *testing.T) {
	for _, line := range []string{"# TYPE 1metric gauge"} {
		_, _, _, err := parsePrometheusTypeLine([]byte(line))
		require.Error(t, err, line)
	}

	name, typ, ok, err := parsePrometheusTypeLine([]byte("# TYPE metric unsupported"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "metric", name)
	require.Equal(t, "unsupported", typ)

	name, typ, ok, err = parsePrometheusTypeLine([]byte("# TYPE metric_info info"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "metric_info", name)
	require.Equal(t, "info", typ)

	_, _, ok, err = parsePrometheusTypeLine([]byte("# HELP metric help"))
	require.NoError(t, err)
	require.False(t, ok)
}
