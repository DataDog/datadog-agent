// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestConvertOldToNewFilter_Success(t *testing.T) {
	tests := []struct {
		name        string
		objectType  workloadfilter.ResourceType
		legacyInput []string
		expected    string
	}{
		{
			"single name filter",
			workloadfilter.ContainerType,
			[]string{"name:foo-.*"},
			`container.name.matches("foo-.*")`,
		},
		{
			"single image filter",
			workloadfilter.ContainerType,
			[]string{"image:nginx.*"},
			`container.image.matches("nginx.*")`,
		},
		{
			"multiple filters",
			workloadfilter.ContainerType,
			[]string{"name:foo-.*", "image:nginx.*"},
			`container.name.matches("foo-.*") || container.image.matches("nginx.*")`,
		},
		{
			"filter with single quote and backslash",
			workloadfilter.ContainerType,
			[]string{`name:foo'bar\zab`},
			`container.name.matches("foo'bar\\zab")`,
		},
		{
			"empty filter is skipped",
			workloadfilter.ContainerType,
			[]string{"", "name:foo"},
			`container.name.matches("foo")`,
		},
		{
			"nil filter is skipped",
			workloadfilter.ContainerType,
			nil,
			"",
		},
		{
			"exclude omitted image key in pod",
			workloadfilter.PodType,
			[]string{"name:foo-.*", "image:nginx.*"},
			`pod.name.matches("foo-.*")`,
		},
		{
			"exclude omitted image key in service",
			workloadfilter.ServiceType,
			[]string{"name:foo-.*", "image:nginx.*"},
			`service.name.matches("foo-.*")`,
		},
		{
			"exclude omitted image key in endpoint",
			workloadfilter.EndpointType,
			[]string{"name:foo-.*", "image:nginx.*"},
			`endpoint.name.matches("foo-.*")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertOldToNewFilter(tt.legacyInput, tt.objectType)
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
			_, err := convertOldToNewFilter(tt.input, workloadfilter.ContainerType)
			assert.Error(t, err)
		})
	}

	t.Run("valid filter key", func(t *testing.T) {
		prog, err := createProgramFromOldFilters([]string{"name:foo-.*"}, workloadfilter.ContainerType)
		assert.NoError(t, err, "should not return an error for valid filter key")
		assert.NotNil(t, prog, "should return a valid program for valid filter key")
	})

	t.Run("invalid filter key", func(t *testing.T) {
		prog, err := createProgramFromOldFilters([]string{"other_field:some_value"}, workloadfilter.ContainerType)
		assert.Error(t, err, "should return an error for invalid filter key")
		assert.Nil(t, prog, "should return a nil program for invalid filter key")
	})
}
