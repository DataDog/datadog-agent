// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateFromSource(t *testing.T) {
	t.Run("parses multiple partitions including synthetic new partition", func(t *testing.T) {
		input := []byte(`package awsrulesfn

var partitions = []Partition{
	{
		ID:          "aws",
		RegionRegex: "^(us|eu)\\-\\w+\\-\\d+$",
		DefaultConfig: PartitionConfig{
			Name:               "aws",
			DnsSuffix:          "amazonaws.com",
			DualStackDnsSuffix: "api.aws",
			SupportsFIPS:       true,
			SupportsDualStack:  true,
		},
		Regions: map[string]RegionOverrides{
			"us-east-1": {
				DnsSuffix:          nil,
				DualStackDnsSuffix: nil,
				SupportsFIPS:       nil,
				SupportsDualStack:  nil,
			},
		},
	},
	{
		ID:          "aws-lunar",
		RegionRegex: "^lunar\\-\\w+\\-\\d+$",
		DefaultConfig: PartitionConfig{
			Name:               "aws-lunar",
			DnsSuffix:          "amazonaws.moon",
			DualStackDnsSuffix: "api.moon.aws",
			SupportsFIPS:       false,
			SupportsDualStack:  true,
		},
	},
}
`)
		out, err := generateFromSource(input)
		require.NoError(t, err)
		got := string(out)
		assert.Contains(t, got, `id:                 "aws"`)
		assert.Contains(t, got, `id:                 "aws-lunar"`)
		assert.Contains(t, got, `dnsSuffix:          "amazonaws.moon"`)
		assert.Contains(t, got, `dualStackDNSSuffix: "api.moon.aws"`)
		assert.Contains(t, got, `supportsFIPS:       false`)
		assert.Contains(t, got, `supportsDualStack:  true`)
	})

	t.Run("fails loudly on non-nil per-region override", func(t *testing.T) {
		input := []byte(`package awsrulesfn

var partitions = []Partition{
	{
		ID:          "aws",
		RegionRegex: "^us\\-\\w+\\-\\d+$",
		DefaultConfig: PartitionConfig{
			DnsSuffix:          "amazonaws.com",
			DualStackDnsSuffix: "api.aws",
			SupportsFIPS:       true,
			SupportsDualStack:  true,
		},
		Regions: map[string]RegionOverrides{
			"us-special-1": {
				DnsSuffix: ptr("custom.amazonaws.com"),
			},
		},
	},
}
`)
		_, err := generateFromSource(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-nil override")
	})

	t.Run("fails when partitions is missing", func(t *testing.T) {
		_, err := generateFromSource([]byte("package awsrulesfn\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "partitions variable not found")
	})

	t.Run("fails when invalid RegionRegex is encountered", func(t *testing.T) {
		input := []byte(`package awsrulesfn

var partitions = []Partition{
	{
		ID:          "aws",
		RegionRegex: "[unclosed",
		DefaultConfig: PartitionConfig{
			DnsSuffix:          "amazonaws.com",
			DualStackDnsSuffix: "api.aws",
		},
	},
}
`)
		_, err := generateFromSource(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid RegionRegex")
	})
}
