// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"

	filter "github.com/DataDog/datadog-agent/comp/core/filter/def"
)

func TestConvertOldToNewFilter_Success(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			"single name filter",
			[]string{"name:foo-.*"},
			`container.name.matches("foo-.*")`,
		},
		{
			"single image filter",
			[]string{"image:nginx.*"},
			`container.image.matches("nginx.*")`,
		},
		{
			"multiple filters",
			[]string{"name:foo-.*", "image:nginx.*"},
			`container.name.matches("foo-.*") || container.image.matches("nginx.*")`,
		},
		{
			"filter with single quote and backslash",
			[]string{`name:foo'bar\zab`},
			`container.name.matches("foo'bar\\zab")`,
		},
		{
			"empty filter is skipped",
			[]string{"", "name:foo"},
			`container.name.matches("foo")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertOldToNewFilter(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestConvertOldToNewFilter_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input []string
	}{
		{
			"missing colon",
			[]string{"namefoo"},
		},
		{
			"unsupported key",
			[]string{"foo:bar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convertOldToNewFilter(tt.input)
			assert.Error(t, err)
		})
	}

	logger := logmock.New(t)
	t.Run("valid filter key", func(t *testing.T) {
		prog := createProgramFromOldFilters([]string{"name:foo-.*"}, filter.ContainerType, logger)
		assert.NotNil(t, prog, "should return a valid program for valid filter key")
	})

	t.Run("invalid filter key", func(t *testing.T) {
		prog := createProgramFromOldFilters([]string{"other_field:some_value"}, filter.ContainerType, logger)
		assert.Nil(t, prog, "should return a nil program for invalid filter key")
	})
}
