// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed dist/host-profiler-config.yaml
var config string

func TestConverterInfraAttributesName(t *testing.T) {
	require.Equal(t, 6, strings.Count(config, "infraattributes/default"))
}
